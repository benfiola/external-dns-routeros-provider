FROM python:3.10.13

WORKDIR /app

ADD external_dns_mikrotik_webhook external_dns_mikrotik_webhook
ADD tests tests
ADD pyproject.toml pyproject.toml
ADD setup.py setup.py

RUN pip install -e .

ENTRYPOINT ["external-dns-mikrotik-webhook", "webhook-server"]
