// Command attractor is the DOT-based pipeline runner and coding agent CLI.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/nigelpepper/attractor/internal/agent"
	"github.com/nigelpepper/attractor/internal/agent/tools"
	"github.com/nigelpepper/attractor/internal/llm/adapters"
	"github.com/nigelpepper/attractor/internal/pipeline"
	"github.com/nigelpepper/attractor/internal/web"
)

const version = "0.1.0"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose int
	root := &cobra.Command{
		Use:           "attractor",
		Short:         "DOT-based directed graph pipeline runner for multi-stage AI workflows.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose == 0 {
				log.SetOutput(io.Discard)
			} else {
				log.SetFlags(log.Ltime)
			}
		},
	}
	root.PersistentFlags().CountVarP(&verbose, "verbose", "v", "Increase verbosity (-v info, -vv debug)")
	root.AddCommand(runCmd(), validateCmd(), chatCmd(), serveCmd())
	return root
}

// signalContext returns a context cancelled on SIGINT/SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// ── run ───────────────────────────────────────────────────────────────────

func runCmd() *cobra.Command {
	var runDir, model, provider, skillsDir, webHost string
	var resume, restartFromSuccess, webEnabled bool
	var webPort int

	cmd := &cobra.Command{
		Use:   "run <dotfile>",
		Short: "Execute a pipeline from a DOT file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dotfile := args[0]
			source, err := os.ReadFile(dotfile)
			if err != nil {
				return fmt.Errorf("file not found: %s", dotfile)
			}

			client := adapters.FromEnv()
			if !client.HasProviders() {
				return fmt.Errorf("no LLM provider configured. Set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
			}

			var skillRegistry *agent.SkillRegistry
			if skillsDir != "" {
				skillRegistry = agent.NewSkillRegistry()
				skillRegistry.LoadDir(skillsDir)
			}

			var restartFrom string
			if restartFromSuccess {
				if runDir == "" {
					return fmt.Errorf("--restart-from-success requires --run-dir pointing at the prior run")
				}
				picked, err := pickRestartNode(runDir)
				if err != nil {
					return err
				}
				restartFrom = picked
			}

			ctx, cancel := signalContext()
			defer cancel()

			tracker := &progressTracker{}
			opts := pipeline.RunnerOptions{
				Client:           client,
				SkillRegistry:    skillRegistry,
				ModelOverride:    model,
				ProviderOverride: provider,
				OnNodeStart:      tracker.onNodeStart,
				OnNodeEnd:        tracker.onNodeEnd,
				OnEdge:           tracker.onEdge,
				OnRetry:          tracker.onRetry,
			}

			// Each run gets a fresh ID; with --web we need it up front so the
			// server can stream this run from its first event.
			runID := pipeline.NewRunID()

			var hub *web.EventHub
			if webEnabled {
				srv := web.NewServer(webRunsDir(runDir), nil)
				hub = srv.Hub()
				hub.SetGraph(runID, string(source))
				cb := web.BridgeRunner(hub, runID)
				opts.OnNodeStart = combineNodeStart(tracker.onNodeStart, cb.OnNodeStart)
				opts.OnNodeEnd = combineNodeEnd(tracker.onNodeEnd, cb.OnNodeEnd)
				opts.OnEdge = combineEdge(tracker.onEdge, cb.OnEdge)
				opts.OnRetry = combineRetry(tracker.onRetry, cb.OnRetry)
				opts.OnAgentEvent = cb.OnAgentEvent

				addr := fmt.Sprintf("%s:%d", webHost, webPort)
				go func() {
					if err := srv.ListenAndServe(ctx, addr); err != nil {
						fmt.Fprintf(os.Stderr, "web server: %s\n", err)
					}
				}()
				fmt.Printf("Web UI: http://%s/?run=%s\n", addr, runID)
			}

			runner := pipeline.NewPipelineRunner(opts)
			result := runner.Run(ctx, string(source), pipeline.RunParams{
				RunDir: runDir, RunID: runID, Resume: resume, RestartFrom: restartFrom,
			})
			if hub != nil {
				hub.MarkFinished(runID)
			}

			fmt.Println()
			if result.Success {
				fmt.Printf("Pipeline succeeded  (run %s)\n", result.RunID)
			} else {
				fmt.Printf("Pipeline failed  (run %s)\n", result.RunID)
				for _, e := range result.Errors {
					fmt.Fprintf(os.Stderr, "  error: %s\n", e)
				}
			}
			fmt.Printf("  nodes executed: %s\n", strings.Join(result.NodesExecuted, ", "))
			fmt.Printf("  run dir: %s\n", result.RunDir)

			// With --web, keep serving after the run so results stay inspectable.
			if hub != nil && ctx.Err() == nil {
				fmt.Printf("\nWeb UI still serving on http://%s:%d  (Ctrl-C to stop)\n", webHost, webPort)
				<-ctx.Done()
			}

			if !result.Success {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&runDir, "run-dir", "", "Directory for run artifacts")
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume from last checkpoint")
	cmd.Flags().BoolVar(&restartFromSuccess, "restart-from-success", false, "Pick a previously successful node and restart from it")
	cmd.Flags().StringVar(&model, "model", "", "Override LLM model (e.g. claude-opus-4-7)")
	cmd.Flags().StringVar(&provider, "provider", "", "Override LLM provider (openai, anthropic, gemini)")
	cmd.Flags().StringVar(&skillsDir, "skills-dir", "", "Directory to load skills from")
	cmd.Flags().BoolVar(&webEnabled, "web", false, "Launch the web UI and stream this run into it")
	cmd.Flags().IntVar(&webPort, "web-port", 8765, "Port for the --web UI")
	cmd.Flags().StringVar(&webHost, "web-host", "127.0.0.1", "Host for the --web UI")
	cmd.MarkFlagsMutuallyExclusive("resume", "restart-from-success")
	return cmd
}

func pickRestartNode(runDir string) (string, error) {
	cp, _ := pipeline.LoadCheckpoint(runDir)
	if cp == nil {
		return "", fmt.Errorf("no checkpoint found in %s", runDir)
	}
	if len(cp.CompletedNodes) == 0 {
		return "", fmt.Errorf("checkpoint in %s has no completed nodes", runDir)
	}
	fmt.Println("Successful nodes (most recent last):")
	for i, nid := range cp.CompletedNodes {
		fmt.Printf("  [%d] %s\n", i+1, nid)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Restart from which node? (number or node id, 'q' to abort): ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("aborted")
		}
		raw := strings.TrimSpace(line)
		switch strings.ToLower(raw) {
		case "q", "quit", "exit":
			return "", fmt.Errorf("aborted")
		}
		if i, err := strconv.Atoi(raw); err == nil && i >= 1 && i <= len(cp.CompletedNodes) {
			return cp.CompletedNodes[i-1], nil
		}
		for _, nid := range cp.CompletedNodes {
			if nid == raw {
				return raw, nil
			}
		}
		fmt.Printf("  invalid choice: %q\n", raw)
	}
}

// ── serve ───────────────────────────────────────────────────────────────────

func serveCmd() *cobra.Command {
	var runsDir, host string
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Launch the web UI over an existing runs/ directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := web.NewServer(runsDir, nil)
			addr := fmt.Sprintf("%s:%d", host, port)
			ctx, cancel := signalContext()
			defer cancel()
			fmt.Printf("Attractor web UI: http://%s/  (Ctrl-C to stop)\n", addr)
			return srv.ListenAndServe(ctx, addr)
		},
	}
	cmd.Flags().StringVar(&runsDir, "runs-dir", "runs", "Directory of prior runs")
	cmd.Flags().IntVar(&port, "port", 8765, "Port")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host")
	return cmd
}

// webRunsDir returns the parent directory the web server should scan, given the
// optional per-run --run-dir override.
func webRunsDir(runDir string) string {
	if runDir == "" {
		return "runs"
	}
	return filepath.Dir(runDir)
}

// callback combinators: the runner takes a single hook each, so we fan out to
// both the terminal progress tracker and the web bridge.
func combineNodeStart(a, b func(*pipeline.Node, int, int)) func(*pipeline.Node, int, int) {
	return func(n *pipeline.Node, i, t int) { a(n, i, t); b(n, i, t) }
}
func combineNodeEnd(a, b func(string, string)) func(string, string) {
	return func(id, s string) { a(id, s); b(id, s) }
}
func combineEdge(a, b func(string, string, string)) func(string, string, string) {
	return func(s, t, l string) { a(s, t, l); b(s, t, l) }
}
func combineRetry(a, b func(string, int, int, float64)) func(string, int, int, float64) {
	return func(id string, at, mx int, d float64) { a(id, at, mx, d); b(id, at, mx, d) }
}

// ── validate ──────────────────────────────────────────────────────────────

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <dotfile>",
		Short: "Validate a DOT pipeline without executing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("file not found: %s", args[0])
			}
			graph, err := pipeline.ParseDOT(string(source))
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}
			diags := pipeline.Validate(graph, nil)

			var errs, warns int
			for _, d := range diags {
				switch d.Severity {
				case pipeline.SeverityError:
					errs++
					fmt.Printf("  ERROR   %s\n", d.Message)
					if d.SuggestedFix != "" {
						fmt.Printf("          fix: %s\n", d.SuggestedFix)
					}
				}
			}
			for _, d := range diags {
				if d.Severity == pipeline.SeverityWarning {
					warns++
					fmt.Printf("  WARN    %s\n", d.Message)
					if d.SuggestedFix != "" {
						fmt.Printf("          fix: %s\n", d.SuggestedFix)
					}
				}
			}

			if errs > 0 {
				fmt.Printf("\n%d error(s), %d warning(s)\n", errs, warns)
				os.Exit(1)
			}
			if warns > 0 {
				fmt.Printf("\nValid with %d warning(s)\n", warns)
			} else {
				fmt.Println("Valid")
			}
			return nil
		},
	}
}

// ── chat ──────────────────────────────────────────────────────────────────

func chatCmd() *cobra.Command {
	var model, provider string
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := adapters.FromEnv()
			if !client.HasProviders() {
				return fmt.Errorf("no LLM provider configured. Set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
			}

			opts := agent.SessionOptions{}
			if model != "" || provider != "" {
				m := model
				if m == "" {
					m = "claude-opus-4-7"
				}
				p := provider
				if p == "" {
					p = "anthropic"
				}
				profile := agent.NewProviderProfile(p, m, "", "")
				config := agent.DefaultSessionConfig(m)
				config.Provider = p
				opts.Profile = &profile
				opts.Config = &config
			}
			opts.Environment = tools.NewLocalEnvironment("")
			session := agent.NewSession(client, opts)

			fmt.Printf("Attractor agent  (model: %s)\n", session.Config().Model)
			fmt.Println("Type /quit to exit.")
			fmt.Println()

			ctx, cancel := signalContext()
			defer cancel()

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("you> ")
				line, err := reader.ReadString('\n')
				if err != nil {
					fmt.Println()
					break
				}
				input := strings.TrimSpace(line)
				if input == "" {
					continue
				}
				if input == "/quit" || input == "/exit" {
					break
				}
				result, err := session.Submit(ctx, input)
				if err != nil {
					fmt.Fprintf(os.Stderr, "\nerror: %s\n\n", err)
					continue
				}
				fmt.Printf("\nagent> %s\n\n", result.FinalResponse)
			}
			session.Close()
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "Override LLM model (e.g. claude-opus-4-7)")
	cmd.Flags().StringVar(&provider, "provider", "", "Override LLM provider (openai, anthropic, gemini)")
	return cmd
}

// ── progress ──────────────────────────────────────────────────────────────

type progressTracker struct{}

func (p *progressTracker) onNodeStart(node *pipeline.Node, index, total int) {
	label := node.Label
	if label == "" {
		label = node.ID
	}
	fmt.Printf("  ▶ [%d/%d] %s (%s)\n", index, total, label, node.Type)
}

func (p *progressTracker) onNodeEnd(nodeID, status string) {
	icon := "✓"
	if status != "success" {
		icon = "✗"
	}
	fmt.Printf("  %s %s → %s\n", icon, nodeID, status)
}

func (p *progressTracker) onEdge(src, target, label string) {
	if label != "" {
		fmt.Printf("  → %s [%s]\n", target, label)
	} else {
		fmt.Printf("  → %s\n", target)
	}
}

func (p *progressTracker) onRetry(nodeID string, attempt, maxRetries int, delay float64) {
	fmt.Printf("  ↻ %s retry %d/%d (wait %.0fs)\n", nodeID, attempt, maxRetries, delay)
}
