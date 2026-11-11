# Answers to the vllm and SGLang command lines the platform renders.
FROM python:3.12-slim
ENV PYTHONPATH=/opt/fake-engine PYTHONUNBUFFERED=1
COPY fake_engine.py /opt/fake-engine/fake_engine.py
COPY sglang /opt/fake-engine/sglang
COPY vllm /usr/local/bin/vllm
