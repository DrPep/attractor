FROM python:3.11-slim

RUN groupadd --gid 1000 attractor \
 && useradd --uid 1000 --gid 1000 --create-home --shell /bin/bash attractor

WORKDIR /opt/attractor
COPY --chown=attractor:attractor pyproject.toml README.md ./
COPY --chown=attractor:attractor attractor ./attractor

RUN pip install --no-cache-dir -e ".[all]"

USER attractor
WORKDIR /sandbox

ENTRYPOINT ["python", "-m", "attractor.cli"]
