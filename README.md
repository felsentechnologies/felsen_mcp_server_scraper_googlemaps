# MCP Server Scraper Google Maps

[English](README.md) | [Portugues](README.pt-BR.md) | [Espanol](README.es.md)

```text
  ______    _                  _____         _                 _             _
 |  ____|  | |                |_   _|       | |               | |           (_)
 | |__ ___ | |___  ___ _ __     | | ___  ___| |__  _ __   ___ | | ___   __ _ _  ___  ___
 |  __/ _ \| / __|/ _ \ '_ \    | |/ _ \/ __| '_ \| '_ \ / _ \| |/ _ \ / _` | |/ _ \/ __|
 | | |  __/| \__ \  __/ | | |   | |  __/ (__| | | | | | | (_) | | (_) | (_| | |  __/\__ \
 |_|  \___||_|___/\___|_| |_|   \_/\___|\___|_| |_|_| |_|\___/|_|\___/ \__, |_|\___||___/
                                                                        __/ |
                                                                       |___/
```

Go MCP server for searching businesses on Google Maps and enriching results with contacts found on official websites.

## Features

- MCP tool `scrape_google_maps`.
- MCP tool `extract_contacts_from_html`.
- Optional HTTP mode with `POST /scrape`, `GET /health`, and remote MCP endpoint `POST /mcp`.
- Google Maps search with headless Chrome.
- Place data extraction: name, address, phone, website, rating, number of reviews, category, image, and Maps URL.
- Optional review content extraction: author, rating, relative date, and text.
- Contact enrichment from websites: emails, phones, and social networks.
- Limited crawling of internal contact/about pages.
- Email and phone deduplication.

## Contact

If you need consulting for deployment, customization, MCP integration, automations, or data collection, get in touch:

- Whatsapp: `55 11 99281-1461`
- E-mail: `lucas.rocha@felsen.enterprises` / `relationship@technologies.felsen.enterprises`

## License

This project uses the `Apache-2.0` license.

Use, copying, modification, distribution, and commercial use are allowed as described in [LICENSE](LICENSE).

This project also includes:

- [NOTICE](NOTICE), with the attribution that must be preserved in redistributions.
- [TRADEMARKS.md](TRADEMARKS.md), with the rules for using the Felsen Technologies trademark.

## How to run

```bash
go mod tidy
go test ./...
go run ./cmd/mcp-googlemaps
```

By default, the binary runs as an MCP server over `stdio`.

To use HTTP mode:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

If you are running locally without Chrome installed, install Chrome/Edge or provide the browser path:

```bash
CHROME_PATH=/usr/bin/chromium go run ./cmd/mcp-googlemaps --http :3000
```

On Windows PowerShell, for example:

```powershell
$env:CHROME_PATH="C:\Program Files\Microsoft\Edge\Application\msedge.exe"
go run .\cmd\mcp-googlemaps --http :3000
```

## Docker

Docker files are in the `Docker/` folder.

Helper development scripts are in `scripts_dev/`:

```bash
chmod +x scripts_dev/linux.sh
./scripts_dev/linux.sh
```

On Windows PowerShell:

```powershell
.\scripts_dev\windows.ps1
```

Copy the sample environment file before starting the stack:

```bash
cp .env.example .env
```

On Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Edit `.env` and replace `HTTP_BEARER_TOKEN` with a strong token.

The `Docker/Dockerfile` installs Chromium inside the image and sets `CHROME_PATH=/usr/bin/chromium`.

```bash
docker build -f Docker/Dockerfile -t mcp-googlemaps .
docker run --rm -p 3000:3000 mcp-googlemaps
```

To start the stack with Docker Compose:

```bash
docker compose -f Docker/docker-compose.yaml up -d --build
```

Variables accepted by compose:

- `HTTP_PORT`: port published on the host. Default: `3000`.
- `HTTP_BEARER_TOKEN`: global bearer token required by all HTTP routes.
- `MCP_BEARER_TOKEN`: fallback for `HTTP_BEARER_TOKEN`, kept for compatibility.
- `MCP_ALLOWED_ORIGINS`: origins allowed for browser calls.
- `DATABASE_URL`: optional PostgreSQL connection string for dataset persistence.
- `TZ`: container timezone. Default: `America/Sao_Paulo`.

Example with bearer token:

```bash
HTTP_BEARER_TOKEN=your-strong-token docker compose -f Docker/docker-compose.yaml up -d --build
```

On Windows PowerShell:

```powershell
$env:HTTP_BEARER_TOKEN="your-strong-token"
docker compose -f Docker/docker-compose.yaml up -d --build
```

Example:

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-strong-token" \
  -d '{"searchQueries":["pizza restaurants in Curitiba"],"maxPlacesPerQuery":3}'
```

## HTTP Routes

HTTP mode is enabled with:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

To use this service as a remote MCP server in OpenAI or another compatible platform, register the public URL of the MCP endpoint:

```text
https://your-domain.com/mcp
```

Local example:

```text
http://localhost:3000/mcp
```

In production, protect the HTTP service with a bearer token:

```bash
HTTP_BEARER_TOKEN=your-strong-token go run ./cmd/mcp-googlemaps --http :3000
```

All HTTP calls to `/health`, `/scrape`, and `/mcp` must send:

```http
Authorization: Bearer your-strong-token
```

For ChatGPT Apps developer mode, the MCP endpoint also accepts API key style headers:

```http
X-API-Key: your-strong-token
```

or:

```http
Api-Key: your-strong-token
```

### GET /health

Checks whether the server is online.

```bash
curl http://localhost:3000/health \
  -H "Authorization: Bearer your-strong-token"
```

Response:

```json
{
  "status": "ok"
}
```

### POST /scrape

Searches businesses on Google Maps and optionally enriches results with emails, phones, social networks, and reviews.

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-strong-token" \
  -d '{
    "searchQueries": ["pizza restaurants in Curitiba"],
    "maxPlacesPerQuery": 20,
    "scrapeEmails": true,
    "scrapePhones": true,
    "scrapeReviews": true,
    "maxReviewsPerPlace": 10,
    "language": "en-US"
  }'
```

Body fields:

- `searchQueries`: list of Google Maps searches. Required; maximum of 10 searches per call.
- `maxPlacesPerQuery`: maximum number of valid businesses per search. Default: `20`; maximum limit: `500`.
- `scrapeEmails`: searches for emails on official websites. Default: `true`.
- `scrapePhones`: searches for phones on official websites. Default: `true`.
- `scrapeReviews`: extracts review content from Google Maps. Default: `false`.
- `maxReviewsPerPlace`: maximum reviews per business when `scrapeReviews` is enabled. Default: `10`; maximum limit: `100`.
- `language`: language used in Google Maps. Default: `pt-BR`.
- `proxyConfiguration.proxyUrls`: optional proxy list. The first proxy is used by Chrome.

Response example:

```json
{
  "count": 1,
  "results": [
    {
      "query": "pizza restaurants in Curitiba",
      "name": "Company name",
      "address": "Example Street, 123 - Curitiba - PR",
      "phone": "(41) 99999-9999",
      "website": "https://example.com",
      "rating": 4.7,
      "reviewsCount": 123,
      "category": "Pizza restaurant",
      "googleMapsUrl": "https://www.google.com/maps/place/...",
      "imageUrl": "https://lh3.googleusercontent.com/...",
      "emails": ["contact@example.com"],
      "phones": ["(41) 99999-9999"],
      "socialLinks": {
        "facebook": null,
        "instagram": "https://www.instagram.com/example",
        "linkedin": null,
        "twitter": null,
        "youtube": null
      },
      "reviews": [
        {
          "author": "Example customer",
          "rating": 5,
          "publishedAt": "2 weeks ago",
          "text": "Great service."
        }
      ]
    }
  ]
}
```

Possible errors:

- `400`: invalid JSON or empty `searchQueries`.
- `401`: missing or invalid bearer token.
- `405`: HTTP method not allowed.
- `413`: request body too large. The `/scrape` body limit is `1 MiB`.
- `503`: bearer token not configured on the server.
- `499`: client canceled the request before completion.
- `504`: timeout reached during scraping.
- `500`: unexpected internal error.

### POST /mcp

Remote MCP endpoint over Streamable HTTP. This is the route that should be registered in OpenAI, Claude, Cursor, Codex, or any client that accepts remote MCP over HTTP.

```text
https://your-domain.com/mcp
```

The endpoint receives MCP JSON-RPC messages by `POST`.

Initialization:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer your-strong-token" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-06-18",
      "capabilities": {},
      "clientInfo": {
        "name": "example-client",
        "version": "1.0.0"
      }
    }
  }'
```

List tools:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer your-strong-token" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

Call the `scrape_google_maps` tool:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer your-strong-token" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "scrape_google_maps",
      "arguments": {
        "searchQueries": ["pizza restaurants in Curitiba"],
        "maxPlacesPerQuery": 20,
        "scrapeEmails": true,
        "scrapePhones": true,
        "scrapeReviews": true,
        "maxReviewsPerPlace": 10,
        "language": "en-US"
      }
    }
  }'
```

Call the `extract_contacts_from_html` tool:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer your-strong-token" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "extract_contacts_from_html",
      "arguments": {
        "html": "<html><body>Contact: contact@example.com</body></html>",
        "baseUrl": "https://example.com"
      }
    }
  }'
```

The `/mcp` endpoint also accepts:

- `ping`
- `resources/list`, returning an empty list
- `prompts/list`, returning an empty list
- notifications, such as `notifications/initialized`, returning `202 Accepted`

The server does not open an SSE stream on `GET /mcp`; for clients that try this mode, it returns `405 Method Not Allowed`. The recommended flow is Streamable HTTP through `POST /mcp`.

#### OpenAI Responses API example

```js
import OpenAI from "openai";

const client = new OpenAI();

const response = await client.responses.create({
  model: "gpt-5",
  tools: [
    {
      type: "mcp",
      server_label: "googlemaps_scraper",
      server_description: "Searches businesses on Google Maps and extracts commercial contacts.",
      server_url: "https://your-domain.com/mcp",
      authorization: process.env.MCP_ACCESS_TOKEN,
      require_approval: "never"
    }
  ],
  input: "Search for 20 pizza restaurants in Curitiba and return name, phone, and website."
});

console.log(response.output_text);
```

#### HTTP Security

For public use, put the service behind HTTPS and configure `HTTP_BEARER_TOKEN`.

When `HTTP_BEARER_TOKEN` is set, all HTTP routes require:

```http
Authorization: Bearer <token>
```

If the token is missing or invalid, the server responds with `401 Unauthorized` and `WWW-Authenticate: Bearer realm="mcp-googlemaps"`.

If neither `HTTP_BEARER_TOKEN` nor `MCP_BEARER_TOKEN` is set, the server fails closed and returns `503 Service Unavailable` for HTTP routes. This avoids publishing `/scrape` or `/mcp` without protection by accident.

`MCP_BEARER_TOKEN` is still accepted as a compatibility fallback, but `HTTP_BEARER_TOKEN` is the recommended name for new installations.

The remote MCP HTTP handler also fails closed when no bearer token is configured. This applies even when `/mcp` is mounted directly outside the bundled HTTP gateway.

The server validates the `Origin` header when it is present. By default, calls without an `Origin` header, same-origin calls, localhost, and OpenAI/ChatGPT origins (`https://chatgpt.com`, `https://chat.openai.com`, `https://platform.openai.com`, `https://developers.openai.com`) are accepted. To allow additional browser origins, configure:

```bash
MCP_ALLOWED_ORIGINS=https://your-app.com,https://another-app.com
```

To allow any origin:

```bash
MCP_ALLOWED_ORIGINS=*
```

In production, prefer listing explicit origins instead of using `*`.

#### Dataset Status

When `DATABASE_URL` is configured, each extraction is persisted with a lifecycle status:

- `running`: extraction started and may still be writing places.
- `finished`: extraction completed successfully.
- `failed`: extraction ended with an error and may include partial results.
- `canceled`: the caller canceled the request and partial results may be present.

The same `status`, `finishedAt`, and `error` fields are written to JSONL extraction records when using a file-backed dataset store.

## MCP Calls

By default, without `--http`, the binary runs as an MCP server over `stdio`:

```bash
go run ./cmd/mcp-googlemaps
```

### initialize

Initializes the MCP session.

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

### notifications/initialized

Notifies that the client completed initialization. This call does not return a response.

```json
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
```

### tools/list

Lists the available tools.

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
```

Returned tools:

- `scrape_google_maps`
- `extract_contacts_from_html`
- `list_dataset_places`
- `list_pending_action_places`
- `get_dataset_place`
- `update_place_actions`
- `append_place_action`

### tools/call: scrape_google_maps

Runs the Google Maps search.

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "scrape_google_maps",
    "arguments": {
      "searchQueries": ["pizza restaurants in Curitiba"],
      "maxPlacesPerQuery": 20,
      "scrapeEmails": true,
      "scrapePhones": true,
      "scrapeReviews": true,
      "maxReviewsPerPlace": 10,
      "language": "en-US"
    }
  }
}
```

### tools/call: extract_contacts_from_html

Extracts emails, phones, social networks, and likely contact page URLs from raw HTML.

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "extract_contacts_from_html",
    "arguments": {
      "html": "<html><body>Contact: contact@example.com</body></html>",
      "baseUrl": "https://example.com"
    }
  }
}
```

### tools/call: list_dataset_places

Lists persisted `dataset_places` records. Requires dataset persistence to be configured.

Useful filters include `query`, `search`, `category`, `minRating`, `maxRating`, `hasReviews`, `pendingActions`, `actionType`, and `missingActionType`.

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "list_dataset_places",
    "arguments": {
      "limit": 50,
      "offset": 0,
      "category": "Pizza",
      "minRating": 4.5,
      "hasReviews": true,
      "pendingActions": true
    }
  }
}
```

### tools/call: get_dataset_place

Gets one persisted place by `id` or `placeKey`.

### tools/call: update_place_actions

Replaces `dataset_places.actions` for one place. `actions` must be a JSON array of objects.

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "update_place_actions",
    "arguments": {
      "placeKey": "name_address:pizza central|rua a, 123",
      "actions": [
        {
          "type": "call",
          "status": "pending",
          "reason": "High rating and recent positive reviews"
        }
      ]
    }
  }
}
```

### tools/call: append_place_action

Appends one action object to `dataset_places.actions` without replacing existing actions. `list_pending_action_places` is a convenience wrapper for places with empty `actions`, or places missing a specific `missingActionType`.
