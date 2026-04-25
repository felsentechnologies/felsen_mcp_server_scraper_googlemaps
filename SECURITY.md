# Security Policy

## Supported Versions

Security updates are provided for the current main branch unless otherwise
stated in a release note.

## Reporting a Vulnerability

Please report security issues privately to Felsen Technologies before opening a
public issue.

Include:

- affected version or commit;
- a clear description of the issue;
- reproduction steps or a proof of concept, when possible;
- any known impact or mitigation.

Do not include secrets, bearer tokens, customer data, or private credentials in
public issues, pull requests, logs, or screenshots.

## Deployment Notes

For public deployments:

- run behind HTTPS;
- configure a strong `HTTP_BEARER_TOKEN`;
- set explicit `MCP_ALLOWED_ORIGINS` values for browser clients;
- rotate tokens if they are exposed;
- keep Docker base images and Go dependencies updated.
