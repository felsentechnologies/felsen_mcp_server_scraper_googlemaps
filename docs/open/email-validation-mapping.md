# Email validation mapping

This document maps practical options for validating emails captured by the Google Maps scraper before an AI agent uses them for outreach, enrichment, or CRM actions.

## Goal

Validate captured emails with enough confidence to decide whether they should be used, reviewed, or discarded.

Recommended output categories:

- `valid`: email is likely deliverable.
- `invalid`: email should not be used.
- `unknown`: provider could not confirm deliverability.
- `catch_all`: domain accepts all addresses; manual or secondary review is recommended.
- `disposable`: temporary mailbox; usually discard.
- `role`: generic mailbox such as `info@`, `contact@`, `admin@`; may be valid but lower priority for personalized outreach.

## Local validation layer

Run these checks before calling an external provider:

- Normalize and trim the email.
- Validate syntax.
- Validate domain format.
- Check DNS MX records.
- Detect disposable email domains from a maintained denylist.
- Detect role accounts such as `info@`, `contato@`, `contact@`, `admin@`, `support@`, `sales@`.
- Deduplicate by normalized email.

Benefits:

- Free and fast.
- Reduces API costs.
- Avoids sending obvious junk to external services.

Limitations:

- Does not prove the mailbox exists.
- Cannot reliably detect catch-all domains without SMTP/provider checks.
- SMTP probing from your own server can be unreliable and may harm IP reputation.

## External provider options

Most reliable email validation APIs require an API key. Truly public unauthenticated APIs are not recommended for production because email validation is abuse-prone and infrastructure-heavy.

### ZeroBounce

Use when:

- You need strong real-time and batch validation.
- You want statuses and substatuses for deliverability, catch-all, abuse, disposable, MX and SMTP-related checks.

Typical fields to store:

- `provider`: `zerobounce`
- `status`
- `subStatus`
- `freeEmail`
- `didYouMean`
- `mxFound`
- `smtpProvider`
- `checkedAt`

Reference:

- https://www.zerobounce.net/docs/email-validation-api-quickstart/v2-validate-emails

### NeverBounce

Use when:

- You want a straightforward deliverability result.
- You need single checks and batch/list workflows.

Typical statuses:

- `valid`
- `invalid`
- `disposable`
- `catchall`
- `unknown`

Reference:

- https://developers.neverbounce.com/docs/verifying-an-email

### Hunter Email Verifier

Use when:

- Your workflow is B2B prospecting.
- You already use Hunter for domain/company discovery.
- You want a score plus signals such as disposable, webmail, accept-all, and SMTP check.

Typical fields to store:

- `provider`: `hunter`
- `status`
- `score`
- `regexp`
- `gibberish`
- `disposable`
- `webmail`
- `mxRecords`
- `smtpServer`
- `smtpCheck`
- `acceptAll`
- `checkedAt`

Reference:

- https://hunter.io/api-documentation

### Abstract Email Validation

Use when:

- You want a simple API response with quality score and deliverability.
- You want quick integration for syntax, DNS, MX, SMTP, disposable, role and catch-all checks.

Typical fields to store:

- `provider`: `abstract`
- `deliverability`
- `qualityScore`
- `isValidFormat`
- `isDisposableEmail`
- `isRoleEmail`
- `isCatchallEmail`
- `isMxFound`
- `isSmtpValid`
- `checkedAt`

Reference:

- https://docs.abstractapi.com/api/email-validation

## Recommended decision policy

Use this policy for the first implementation:

| Signal | Decision | Notes |
| --- | --- | --- |
| Syntax invalid | `invalid` | Do not call external API. |
| No MX records | `invalid` | Domain cannot receive mail normally. |
| Disposable domain | `disposable` | Usually discard. |
| Provider says valid | `valid` | Safe for next workflow step. |
| Provider says invalid | `invalid` | Do not use. |
| Provider says catch-all | `catch_all` | Keep, but mark for review or low-confidence outreach. |
| Provider says unknown | `unknown` | Retry later or route to review. |
| Role mailbox | `role` | Valid for business contact, lower priority for personalized outreach. |

## Suggested MCP integration

Add a tool:

- `validate_emails`

Suggested input:

```json
{
  "emails": ["contact@example.com"],
  "provider": "local",
  "persist": false
}
```

Supported providers:

- `local`
- `zerobounce`
- `neverbounce`
- `hunter`
- `abstract`

Suggested output:

```json
{
  "count": 1,
  "results": [
    {
      "email": "contact@example.com",
      "normalizedEmail": "contact@example.com",
      "status": "valid",
      "confidence": 0.95,
      "provider": "zerobounce",
      "signals": {
        "syntaxValid": true,
        "mxFound": true,
        "disposable": false,
        "role": true,
        "catchAll": false
      },
      "checkedAt": "2026-04-26T00:00:00Z"
    }
  ]
}
```

Add a second tool for persisted places:

- `validate_place_emails`

Suggested input:

```json
{
  "placeId": 123,
  "placeKey": "name_address:pizza central|rua a, 123",
  "provider": "zerobounce",
  "updateActions": true
}
```

When `updateActions` is true, append an action to `dataset_places.actions`:

```json
{
  "type": "email_validation",
  "status": "finished",
  "provider": "zerobounce",
  "summary": {
    "valid": 2,
    "invalid": 1,
    "unknown": 0,
    "catchAll": 1
  },
  "checkedAt": "2026-04-26T00:00:00Z"
}
```

## Recommended environment variables

```bash
EMAIL_VALIDATION_PROVIDER=local
ZEROBOUNCE_API_KEY=
NEVERBOUNCE_API_KEY=
HUNTER_API_KEY=
ABSTRACT_API_KEY=
```

Rules:

- Do not log API keys.
- Do not store provider raw responses unless needed.
- Store a normalized validation summary in `actions` or in a future dedicated table.
- Prefer batching when validating many emails.

## Implementation priority

1. Add local syntax, domain, MX, disposable and role checks.
2. Add `validate_emails` MCP tool.
3. Add provider interface and one paid provider, preferably ZeroBounce or NeverBounce.
4. Add `validate_place_emails` to read emails from `dataset_places`.
5. Persist validation summary into `dataset_places.actions`.
6. Add rate limiting, caching and retry policy.
