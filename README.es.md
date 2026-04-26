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

Servidor MCP en Go para buscar empresas en Google Maps y enriquecer los resultados con contactos encontrados en los sitios web oficiales.

## Funcionalidades

- Herramienta MCP `scrape_google_maps`.
- Herramienta MCP `extract_contacts_from_html`.
- Modo HTTP opcional con `POST /scrape`, `GET /health` y endpoint MCP remoto `POST /mcp`.
- Busqueda en Google Maps con Chrome headless.
- Extraccion de datos del lugar: nombre, direccion, telefono, sitio web, calificacion, numero de resenas, categoria, imagen y URL de Maps.
- Extraccion opcional del contenido de resenas: autor, calificacion, fecha relativa y texto.
- Enriquecimiento de contactos en sitios web: emails, telefonos y redes sociales.
- Busqueda limitada en paginas internas de contacto/sobre nosotros.
- Deduplicacion de emails y telefonos.

## Contacto

Si necesitas consultoria para despliegue, personalizacion, integracion MCP, automatizaciones o recoleccion de datos, ponte en contacto:

- Whatsapp: `55 11 99281-1461`
- E-mail: `lucas.rocha@felsen.enterprises` / `relationship@technologies.felsen.enterprises`

## Licencia

Este proyecto usa la licencia `Apache-2.0`.

El uso, copia, modificacion, distribucion y uso comercial estan permitidos segun lo descrito en [LICENSE](LICENSE).

Este proyecto tambien incluye:

- [NOTICE](NOTICE), con la atribucion que debe preservarse en redistribuciones.
- [TRADEMARKS.md](TRADEMARKS.md), con las reglas de uso de la marca Felsen Technologies.

## Como ejecutar

```bash
go mod tidy
go test ./...
go run ./cmd/mcp-googlemaps
```

Por defecto, el binario se ejecuta como servidor MCP por `stdio`.

Para usar el modo HTTP:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

Si estas ejecutando localmente sin Chrome instalado, instala Chrome/Edge o informa la ruta del navegador:

```bash
CHROME_PATH=/usr/bin/chromium go run ./cmd/mcp-googlemaps --http :3000
```

En Windows PowerShell, por ejemplo:

```powershell
$env:CHROME_PATH="C:\Program Files\Microsoft\Edge\Application\msedge.exe"
go run .\cmd\mcp-googlemaps --http :3000
```

## Docker

Los archivos Docker estan en la carpeta `Docker/`.

Los scripts auxiliares de desarrollo estan en `scripts_dev/`:

```bash
chmod +x scripts_dev/linux.sh
./scripts_dev/linux.sh
```

En Windows PowerShell:

```powershell
.\scripts_dev\windows.ps1
```

Copia el archivo de entorno de ejemplo antes de iniciar la stack:

```bash
cp .env.example .env
```

En Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Edita `.env` y reemplaza `HTTP_BEARER_TOKEN` por un token fuerte.

El `Docker/Dockerfile` instala Chromium dentro de la imagen y configura `CHROME_PATH=/usr/bin/chromium`.

```bash
docker build -f Docker/Dockerfile -t mcp-googlemaps .
docker run --rm -p 3000:3000 mcp-googlemaps
```

Para iniciar la stack con Docker Compose:

```bash
docker compose -f Docker/docker-compose.yaml up -d --build
```

Variables aceptadas por compose:

- `HTTP_PORT`: puerto publicado en el host. Valor por defecto: `3000`.
- `HTTP_BEARER_TOKEN`: bearer token global requerido por todas las rutas HTTP.
- `MCP_BEARER_TOKEN`: fallback para `HTTP_BEARER_TOKEN`, mantenido por compatibilidad.
- `MCP_ALLOWED_ORIGINS`: origenes permitidos para llamadas desde navegador.
- `DATABASE_URL`: string de conexion PostgreSQL opcional para persistencia del dataset.
- `TZ`: zona horaria del container. Valor por defecto: `America/Sao_Paulo`.

Ejemplo con bearer token:

```bash
HTTP_BEARER_TOKEN=tu-token-fuerte docker compose -f Docker/docker-compose.yaml up -d --build
```

En Windows PowerShell:

```powershell
$env:HTTP_BEARER_TOKEN="tu-token-fuerte"
docker compose -f Docker/docker-compose.yaml up -d --build
```

Ejemplo:

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -d '{"searchQueries":["pizzerias en Curitiba"],"maxPlacesPerQuery":3}'
```

## Rutas HTTP

El modo HTTP se habilita con:

```bash
go run ./cmd/mcp-googlemaps --http :3000
```

Para usar este servicio como MCP remoto en OpenAI o en otra plataforma compatible, registra la URL publica del endpoint MCP:

```text
https://tu-dominio.com/mcp
```

Ejemplo local:

```text
http://localhost:3000/mcp
```

En produccion, protege el servicio HTTP con bearer token:

```bash
HTTP_BEARER_TOKEN=tu-token-fuerte go run ./cmd/mcp-googlemaps --http :3000
```

Todas las llamadas HTTP a `/health`, `/scrape` y `/mcp` deben enviar:

```http
Authorization: Bearer tu-token-fuerte
```

### GET /health

Verifica si el servidor esta online.

```bash
curl http://localhost:3000/health \
  -H "Authorization: Bearer tu-token-fuerte"
```

Respuesta:

```json
{
  "status": "ok"
}
```

### POST /scrape

Busca empresas en Google Maps y, opcionalmente, enriquece los resultados con emails, telefonos, redes sociales y resenas.

```bash
curl -X POST http://localhost:3000/scrape \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -d '{
    "searchQueries": ["pizzerias en Curitiba"],
    "maxPlacesPerQuery": 20,
    "scrapeEmails": true,
    "scrapePhones": true,
    "scrapeReviews": true,
    "maxReviewsPerPlace": 10,
    "language": "es-ES"
  }'
```

Campos del cuerpo:

- `searchQueries`: lista de busquedas en Google Maps. Obligatorio; maximo de 10 busquedas por llamada.
- `maxPlacesPerQuery`: maximo de empresas validas por busqueda. Valor por defecto: `20`; limite maximo: `500`.
- `scrapeEmails`: busca emails en los sitios web oficiales. Valor por defecto: `true`.
- `scrapePhones`: busca telefonos en los sitios web oficiales. Valor por defecto: `true`.
- `scrapeReviews`: extrae el contenido de resenas en Google Maps. Valor por defecto: `false`.
- `maxReviewsPerPlace`: maximo de resenas por empresa cuando `scrapeReviews` esta activo. Valor por defecto: `10`; limite maximo: `100`.
- `language`: idioma usado en Google Maps. Valor por defecto: `pt-BR`.
- `proxyConfiguration.proxyUrls`: lista opcional de proxies. Chrome usa el primer proxy.

Ejemplo de respuesta:

```json
{
  "count": 1,
  "results": [
    {
      "query": "pizzerias en Curitiba",
      "name": "Nombre de la empresa",
      "address": "Calle Ejemplo, 123 - Curitiba - PR",
      "phone": "(41) 99999-9999",
      "website": "https://example.com",
      "rating": 4.7,
      "reviewsCount": 123,
      "category": "Pizzeria",
      "googleMapsUrl": "https://www.google.com/maps/place/...",
      "imageUrl": "https://lh3.googleusercontent.com/...",
      "emails": ["contacto@example.com"],
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
          "author": "Cliente Ejemplo",
          "rating": 5,
          "publishedAt": "hace 2 semanas",
          "text": "Excelente atencion."
        }
      ]
    }
  ]
}
```

Posibles errores:

- `400`: JSON invalido o `searchQueries` vacio.
- `401`: bearer token ausente o invalido.
- `405`: metodo HTTP no permitido.
- `413`: cuerpo de la solicitud demasiado grande. El limite de `/scrape` es `1 MiB`.
- `503`: bearer token no configurado en el servidor.
- `499`: el cliente cancelo la solicitud antes de finalizar.
- `504`: tiempo limite alcanzado durante el scraping.
- `500`: error interno inesperado.

### POST /mcp

Endpoint MCP remoto via Streamable HTTP. Esta es la ruta que debe registrarse en OpenAI, Claude, Cursor, Codex o cualquier cliente que acepte MCP remoto por HTTP.

```text
https://tu-dominio.com/mcp
```

El endpoint recibe mensajes JSON-RPC MCP por `POST`.

Inicializacion:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-11-25",
      "capabilities": {},
      "clientInfo": {
        "name": "example-client",
        "version": "1.0.0"
      }
    }
  }'
```

Listar herramientas:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -H "MCP-Protocol-Version: 2025-11-25" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

Llamar la herramienta `scrape_google_maps`:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -H "MCP-Protocol-Version: 2025-11-25" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "scrape_google_maps",
      "arguments": {
        "searchQueries": ["pizzerias en Curitiba"],
        "maxPlacesPerQuery": 20,
        "scrapeEmails": true,
        "scrapePhones": true,
        "scrapeReviews": true,
        "maxReviewsPerPlace": 10,
        "language": "es-ES"
      }
    }
  }'
```

Llamar la herramienta `extract_contacts_from_html`:

```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Authorization: Bearer tu-token-fuerte" \
  -H "MCP-Protocol-Version: 2025-11-25" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "extract_contacts_from_html",
      "arguments": {
        "html": "<html><body>Contacto: contacto@example.com</body></html>",
        "baseUrl": "https://example.com"
      }
    }
  }'
```

El endpoint `/mcp` tambien acepta:

- `ping`
- `resources/list`, devuelve una lista vacia
- `prompts/list`, devuelve una lista vacia
- notificaciones, como `notifications/initialized`, respondiendo `202 Accepted`

El servidor no abre stream SSE en `GET /mcp`; para clientes que intentan este modo, responde `405 Method Not Allowed`. El flujo recomendado es Streamable HTTP por `POST /mcp`.

#### Ejemplo en OpenAI Responses API

```js
import OpenAI from "openai";

const client = new OpenAI();

const response = await client.responses.create({
  model: "gpt-5",
  tools: [
    {
      type: "mcp",
      server_label: "googlemaps_scraper",
      server_description: "Busca empresas en Google Maps y extrae contactos comerciales.",
      server_url: "https://tu-dominio.com/mcp",
      authorization: process.env.MCP_ACCESS_TOKEN,
      require_approval: "never"
    }
  ],
  input: "Busca 20 pizzerias en Curitiba y devuelve nombre, telefono y sitio web."
});

console.log(response.output_text);
```

#### Seguridad HTTP

Para uso publico, coloca el servicio detras de HTTPS y configura `HTTP_BEARER_TOKEN`.

Cuando `HTTP_BEARER_TOKEN` esta definido, todas las rutas HTTP requieren:

```http
Authorization: Bearer <token>
```

Para el developer mode de ChatGPT Apps, el endpoint MCP tambien acepta headers estilo API key:

```http
X-API-Key: <token>
```

o:

```http
Api-Key: <token>
```

Si el token esta ausente o es invalido, el servidor responde `401 Unauthorized` con `WWW-Authenticate: Bearer realm="mcp-googlemaps"`.

Si `HTTP_BEARER_TOKEN` y `MCP_BEARER_TOKEN` no estan definidos, el servidor falla cerrado y responde `503 Service Unavailable` para rutas HTTP. Esto evita publicar `/scrape` o `/mcp` sin proteccion por accidente.

`MCP_BEARER_TOKEN` todavia se acepta como fallback de compatibilidad, pero `HTTP_BEARER_TOKEN` es el nombre recomendado para nuevas instalaciones.

El handler MCP HTTP remoto tambien falla cerrado cuando ningun bearer token esta configurado. Esto aplica incluso cuando `/mcp` se monta directamente fuera del gateway HTTP incluido.

El servidor valida el header `Origin` cuando esta presente. Por defecto, se aceptan llamadas sin header `Origin`, mismo origen, localhost y origins OpenAI/ChatGPT (`https://chatgpt.com`, `https://chat.openai.com`, `https://platform.openai.com`, `https://developers.openai.com`). Para liberar origenes de navegador adicionales, configura:

```bash
MCP_ALLOWED_ORIGINS=https://tu-app.com,https://otra-app.com
```

Para liberar cualquier origen:

```bash
MCP_ALLOWED_ORIGINS=*
```

En produccion, prefiere listar origenes explicitos en vez de usar `*`.

#### Status del Dataset

Cuando `DATABASE_URL` esta configurado, cada extraccion se persiste con status de ciclo de vida:

- `running`: la extraccion inicio y todavia puede estar grabando lugares.
- `finished`: la extraccion termino correctamente.
- `failed`: la extraccion termino con error y puede incluir resultados parciales.
- `canceled`: el cliente cancelo la solicitud y puede haber resultados parciales.

Los mismos campos `status`, `finishedAt` y `error` se escriben en los registros JSONL de extraccion cuando se usa un store de dataset en archivo.

## Llamadas MCP

Por defecto, sin `--http`, el binario se ejecuta como servidor MCP por `stdio`:

```bash
go run ./cmd/mcp-googlemaps
```

### initialize

Inicializa la sesion MCP.

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
```

### notifications/initialized

Notifica que el cliente completo la inicializacion. Esta llamada no devuelve respuesta.

```json
{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}
```

### tools/list

Lista las herramientas disponibles.

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
```

Herramientas devueltas:

- `scrape_google_maps`
- `extract_contacts_from_html`
- `list_dataset_places`
- `list_pending_action_places`
- `get_dataset_place`
- `update_place_actions`
- `append_place_action`

### tools/call: scrape_google_maps

Ejecuta la busqueda en Google Maps.

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "scrape_google_maps",
    "arguments": {
      "searchQueries": ["pizzerias en Curitiba"],
      "maxPlacesPerQuery": 20,
      "scrapeEmails": true,
      "scrapePhones": true,
      "scrapeReviews": true,
      "maxReviewsPerPlace": 10,
      "language": "es-ES"
    }
  }
}
```

### tools/call: extract_contacts_from_html

Extrae emails, telefonos, redes sociales y URLs probables de paginas de contacto a partir de HTML bruto.

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "extract_contacts_from_html",
    "arguments": {
      "html": "<html><body>Contacto: contacto@example.com</body></html>",
      "baseUrl": "https://example.com"
    }
  }
}
```

### tools/call: list_dataset_places

Lista registros persistidos de `dataset_places`. Requiere persistencia de dataset configurada.

Filtros utiles incluyen `query`, `search`, `category`, `minRating`, `maxRating`, `hasReviews`, `pendingActions`, `actionType` y `missingActionType`.

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

Obtiene un lugar persistido por `id` o `placeKey`.

### tools/call: update_place_actions

Reemplaza `dataset_places.actions` de un lugar. `actions` debe ser un array JSON de objetos.

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
          "reason": "Alta calificacion y resenas positivas recientes"
        }
      ]
    }
  }
}
```

### tools/call: append_place_action

Agrega una action al array `dataset_places.actions` sin reemplazar las existentes. `list_pending_action_places` es un atajo para lugares con `actions` vacio, o lugares sin un `missingActionType` especifico.
