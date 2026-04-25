# MCP Server Scraper Google Maps

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

Servidor MCP em Go para buscar empresas no Google Maps e enriquecer os resultados com contatos encontrados nos websites oficiais.

## Funcionalidades

- Ferramenta MCP `scrape_google_maps`.
- Ferramenta MCP `extract_contacts_from_html`.
- Modo HTTP opcional com `POST /scrape`, `GET /health` e endpoint MCP remoto `POST /mcp`.
- Busca no Google Maps com Chrome headless.
- Extracao de dados do lugar: nome, endereco, telefone, website, avaliacao, numero de reviews, categoria, imagem e URL do Maps.
- Enriquecimento de contatos em websites: emails, telefones e redes sociais.
- Busca limitada por paginas internas de contato/sobre.
- Deduplicacao de emails e telefones.

## Licenca

Este projeto usa a licenca `Apache-2.0`.

O uso, copia, modificacao, distribuicao e uso comercial sao permitidos conforme descrito no arquivo [LICENSE](LICENSE).

Este projeto tambem inclui:

- [NOTICE](NOTICE), com a atribuicao que deve ser preservada em redistribuicoes.
- [TRADEMARKS.md](TRADEMARKS.md), com as regras de uso da marca Felsen Technologies.

## Como rodar

```bash
go mod tidy
go test ./...
go run ./cmd/mcp-googlemaps
```

Por padrao o binario roda como servidor MCP por `stdio`.

Para usar o modo HTTP:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

Se estiver rodando localmente sem Chrome instalado, instale Chrome/Edge ou informe o caminho do browser:

```bash
CHROME_PATH=/usr/bin/chromium go run ./cmd/mcp-googlemaps --http :3000
```

No Windows PowerShell, por exemplo:

```powershell
$env:CHROME_PATH="C:\Program Files\Microsoft\Edge\Application\msedge.exe"
go run .\cmd\mcp-googlemaps --http :3000
```

## Docker

Os arquivos Docker ficam na pasta `Docker/`.

Scripts auxiliares para desenvolvimento ficam em `scripts_dev/`:

```bash
chmod +x scripts_dev/linux.sh
./scripts_dev/linux.sh
```

No Windows PowerShell:

```powershell
.\scripts_dev\windows.ps1
```

Copie o arquivo de exemplo de ambiente antes de subir a stack:

```bash
cp .env.example .env
```

No Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Edite o `.env` e substitua `HTTP_BEARER_TOKEN` por um token forte.

O `Docker/Dockerfile` instala Chromium dentro da imagem e configura `CHROME_PATH=/usr/bin/chromium`.

```bash
docker build -f Docker/Dockerfile -t mcp-googlemaps .
docker run --rm -p 3000:3000 mcp-googlemaps
```

Para subir a stack com Docker Compose:

```bash
docker compose -f Docker/docker-compose.yaml up -d --build
```

Variaveis aceitas pelo compose:

- `HTTP_PORT`: porta publicada no host. Padrao: `3000`.
- `HTTP_BEARER_TOKEN`: bearer token global exigido por todas as rotas HTTP.
- `MCP_BEARER_TOKEN`: fallback para `HTTP_BEARER_TOKEN`, mantido por compatibilidade.
- `MCP_ALLOWED_ORIGINS`: origens permitidas para chamadas browser.
- `TZ`: timezone do container. Padrao: `America/Sao_Paulo`.

Exemplo com bearer token:

```bash
HTTP_BEARER_TOKEN=seu-token-forte docker compose -f Docker/docker-compose.yaml up -d --build
```

No Windows PowerShell:

```powershell
$env:HTTP_BEARER_TOKEN="seu-token-forte"
docker compose -f Docker/docker-compose.yaml up -d --build
```

Exemplo:

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer seu-token-forte" \
  -d '{"searchQueries":["pizzarias em Curitiba"],"maxPlacesPerQuery":3}'
```

## Rotas HTTP

O modo HTTP e habilitado com:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

Para usar este servico como MCP remoto na OpenAI ou em outra plataforma compativel, cadastre a URL publica do endpoint MCP:

```text
https://seu-dominio.com/mcp
```

Exemplo local:

```text
http://localhost:3000/mcp
```

Em producao, proteja o servico HTTP com bearer token:

```bash
HTTP_BEARER_TOKEN=seu-token-forte go run ./cmd/mcp-googlemaps --http :3000
```

Todas as chamadas HTTP para `/health`, `/scrape` e `/mcp` deverao enviar:

```http
Authorization: Bearer seu-token-forte
```

### GET /health

Verifica se o servidor esta online.

```bash
curl http://localhost:3000/health \
  -H "Authorization: Bearer seu-token-forte"
```

Resposta:

```json
{
  "status": "ok"
}
```

### POST /scrape

Busca empresas no Google Maps e, opcionalmente, enriquece os resultados com emails, telefones e redes sociais encontrados nos websites oficiais.

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer seu-token-forte" \
  -d '{
    "searchQueries": ["pizzarias em Curitiba"],
    "maxPlacesPerQuery": 20,
    "scrapeEmails": true,
    "scrapePhones": true,
    "language": "pt-BR"
  }'
```

Campos do corpo:

- `searchQueries`: lista de buscas no Google Maps. Obrigatorio; maximo de 10 buscas por chamada.
- `maxPlacesPerQuery`: maximo de empresas validas por busca. Padrao: `20`; limite maximo: `500`.
- `scrapeEmails`: busca emails nos websites oficiais. Padrao: `true`.
- `scrapePhones`: busca telefones nos websites oficiais. Padrao: `true`.
- `language`: idioma usado no Google Maps. Padrao: `pt-BR`.
- `proxyConfiguration.proxyUrls`: lista opcional de proxies. O primeiro proxy e usado pelo Chrome.

Exemplo de resposta:

```json
{
  "count": 1,
  "results": [
    {
      "query": "pizzarias em Curitiba",
      "name": "Nome da empresa",
      "address": "Rua Exemplo, 123 - Curitiba - PR",
      "phone": "(41) 99999-9999",
      "website": "https://example.com",
      "rating": 4.7,
      "reviewsCount": 123,
      "category": "Pizzaria",
      "googleMapsUrl": "https://www.google.com/maps/place/...",
      "imageUrl": "https://lh3.googleusercontent.com/...",
      "emails": ["contato@example.com"],
      "phones": ["(41) 99999-9999"],
      "socialLinks": {
        "facebook": null,
        "instagram": "https://www.instagram.com/example",
        "linkedin": null,
        "twitter": null,
        "youtube": null
      }
    }
  ]
}
```

Possiveis erros:

- `400`: JSON invalido ou `searchQueries` vazio.
- `401`: token bearer ausente ou invalido.
- `405`: metodo HTTP nao permitido.
- `503`: token bearer nao configurado no servidor.
- `499`: cliente cancelou a requisicao antes do fim.
- `504`: tempo limite atingido durante a raspagem.
- `500`: erro interno inesperado.

### POST /mcp

Endpoint MCP remoto via Streamable HTTP. Esta e a rota que deve ser cadastrada na OpenAI, Claude, Cursor, Codex ou qualquer cliente que aceite MCP remoto por HTTP.

```text
https://seu-dominio.com/mcp
```

O endpoint recebe mensagens JSON-RPC MCP por `POST`.

Inicializacao:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer seu-token-forte" \
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

Listar ferramentas:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer seu-token-forte" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

Chamar a ferramenta `scrape_google_maps`:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer seu-token-forte" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "scrape_google_maps",
      "arguments": {
        "searchQueries": ["pizzarias em Curitiba"],
        "maxPlacesPerQuery": 20,
        "scrapeEmails": true,
        "scrapePhones": true,
        "language": "pt-BR"
      }
    }
  }'
```

Chamar a ferramenta `extract_contacts_from_html`:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer seu-token-forte" \
  -H "MCP-Protocol-Version: 2025-06-18" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "extract_contacts_from_html",
      "arguments": {
        "html": "<html><body>Contato: contato@example.com</body></html>",
        "baseUrl": "https://example.com"
      }
    }
  }'
```

O endpoint `/mcp` tambem aceita:

- `ping`
- `resources/list`, retornando lista vazia
- `prompts/list`, retornando lista vazia
- notificacoes, como `notifications/initialized`, respondendo `202 Accepted`

O servidor nao abre stream SSE em `GET /mcp`; para clientes que tentam esse modo, ele responde `405 Method Not Allowed`. O fluxo recomendado e Streamable HTTP por `POST /mcp`.

#### Exemplo na OpenAI Responses API

```js
import OpenAI from "openai";

const client = new OpenAI();

const response = await client.responses.create({
  model: "gpt-5",
  tools: [
    {
      type: "mcp",
      server_label: "googlemaps_scraper",
      server_description: "Busca empresas no Google Maps e extrai contatos comerciais.",
      server_url: "https://seu-dominio.com/mcp",
      authorization: process.env.MCP_ACCESS_TOKEN,
      require_approval: "never"
    }
  ],
  input: "Busque 20 pizzarias em Curitiba e retorne nome, telefone e website."
});

console.log(response.output_text);
```

#### Seguranca HTTP

Para uso publico, coloque o servico atras de HTTPS e configure `HTTP_BEARER_TOKEN`.

Quando `HTTP_BEARER_TOKEN` esta definido, todas as rotas HTTP exigem:

```http
Authorization: Bearer <token>
```

Se o token estiver ausente ou invalido, o servidor responde `401 Unauthorized` com `WWW-Authenticate: Bearer realm="mcp-googlemaps"`.

Se `HTTP_BEARER_TOKEN` e `MCP_BEARER_TOKEN` nao estiverem definidos, o servidor falha fechado e responde `503 Service Unavailable` para rotas HTTP. Isso evita publicar `/scrape` ou `/mcp` sem protecao por acidente.

`MCP_BEARER_TOKEN` ainda e aceito como fallback para compatibilidade, mas `HTTP_BEARER_TOKEN` e o nome recomendado para novas instalacoes.

O servidor valida o header `Origin` quando ele estiver presente. Por padrao, chamadas sem header `Origin`, mesma origem e localhost sao aceitas. Para liberar origens de browser especificas, configure:

```bash
MCP_ALLOWED_ORIGINS=https://seu-app.com,https://outro-app.com
```

Para liberar qualquer origem:

```bash
MCP_ALLOWED_ORIGINS=*
```

Em producao, prefira listar origens explicitas em vez de usar `*`.

## Chamadas MCP

Por padrao, sem `--http`, o binario roda como servidor MCP por `stdio`:

```bash
go run ./cmd/mcp-googlemaps
```

### initialize

Inicializa a sessao MCP.

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

### notifications/initialized

Notifica que o cliente concluiu a inicializacao. Esta chamada nao retorna resposta.

```json
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
```

### tools/list

Lista as ferramentas disponiveis.

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
```

Ferramentas retornadas:

- `scrape_google_maps`
- `extract_contacts_from_html`

### tools/call: scrape_google_maps

Executa a busca no Google Maps.

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "scrape_google_maps",
    "arguments": {
      "searchQueries": ["pizzarias em Curitiba"],
      "maxPlacesPerQuery": 20,
      "scrapeEmails": true,
      "scrapePhones": true,
      "language": "pt-BR"
    }
  }
}
```

### tools/call: extract_contacts_from_html

Extrai emails, telefones, redes sociais e URLs provaveis de paginas de contato a partir de um HTML bruto.

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "extract_contacts_from_html",
    "arguments": {
      "html": "<html><body>Contato: contato@example.com</body></html>",
      "baseUrl": "https://example.com"
    }
  }
}
```
