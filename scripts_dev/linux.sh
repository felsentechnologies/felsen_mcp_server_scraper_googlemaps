#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="mcp-googlemaps"
HOST_PORT="3000"
BEARER_TOKEN=""
ALLOWED_ORIGINS=""
DATABASE_URL="${DATABASE_URL:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image-name)
      IMAGE_NAME="${2:-}"
      shift 2
      ;;
    --host-port)
      HOST_PORT="${2:-}"
      shift 2
      ;;
    --bearer-token)
      BEARER_TOKEN="${2:-}"
      shift 2
      ;;
    --allowed-origins)
      ALLOWED_ORIGINS="${2:-}"
      shift 2
      ;;
    --database-url)
      DATABASE_URL="${2:-}"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [--image-name NAME] [--host-port PORT] [--bearer-token TOKEN] [--allowed-origins ORIGINS] [--database-url URL]"
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$PROJECT_ROOT/Docker/docker-compose.yaml"
TOKEN_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/FelsenTechnologies/mcp-googlemaps"
TOKEN_FILE="$TOKEN_DIR/bearer-token.txt"
GENERATED_BEARER_TOKEN="false"
LAST_COMPOSE_SUCCEEDED="false"

write_title() {
  printf "\n=== %s ===\n" "$1"
}

write_felsen_banner() {
  cat <<'EOF'
  ______    _                  _____         _                 _             _
 |  ____|  | |                |_   _|       | |               | |           (_)
 | |__ ___ | |___  ___ _ __     | | ___  ___| |__  _ __   ___ | | ___   __ _ _  ___  ___
 |  __/ _ \| / __|/ _ \ '_ \    | |/ _ \/ __| '_ \| '_ \ / _ \| |/ _ \ / _` | |/ _ \/ __|
 | | |  __/| \__ \  __/ | | |   | |  __/ (__| | | | | | | (_) | | (_) | (_| | |  __/\__ \
 |_|  \___||_|___/\___|_| |_|   \_/\___|\___|_| |_|_| |_|\___/|_|\___/ \__, |_|\___||___/
                                                                        __/ |
                                                                       |___/
EOF
}

test_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "Docker nao esta instalado ou nao esta no PATH." >&2
    return 1
  fi
  if ! docker version >/dev/null 2>&1; then
    echo "Docker nao esta respondendo. Inicie o daemon do Docker e tente novamente." >&2
    return 1
  fi
}

new_bearer_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
    return
  fi
  if [[ -r /dev/urandom ]]; then
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
    return
  fi
  date +%s%N | sha256sum | awk '{print $1}'
}

ensure_bearer_token() {
  if [[ -n "${BEARER_TOKEN// }" ]]; then
    return
  fi

  if [[ -f "$TOKEN_FILE" ]]; then
    local saved_token
    saved_token="$(tr -d '\r\n' < "$TOKEN_FILE")"
    if [[ -n "${saved_token// }" ]]; then
      BEARER_TOKEN="$saved_token"
      GENERATED_BEARER_TOKEN="false"
      return
    fi
  fi

  BEARER_TOKEN="$(new_bearer_token)"
  GENERATED_BEARER_TOKEN="true"
  mkdir -p "$TOKEN_DIR"
  printf "%s" "$BEARER_TOKEN" > "$TOKEN_FILE"
  chmod 600 "$TOKEN_FILE" 2>/dev/null || true
}

write_bearer_token_info() {
  if [[ -z "${BEARER_TOKEN// }" ]]; then
    return
  fi

  printf "\n"
  if [[ "$GENERATED_BEARER_TOKEN" == "true" ]]; then
    echo "HTTP_BEARER_TOKEN gerado automaticamente:"
  else
    echo "HTTP_BEARER_TOKEN configurado:"
  fi
  echo "$BEARER_TOKEN"
  printf "\nUse este valor na OpenAI como authorization/bearer token.\n"
}

write_database_url_info() {
  printf "\n"
  if [[ -z "${DATABASE_URL// }" ]]; then
    echo "DATABASE_URL nao configurado. A stack subiu sem persistencia em database."
    return
  fi

  echo "DATABASE_URL configurado. O servidor ira migrar e usar a database dataset ao iniciar."
}

invoke_compose() {
  LAST_COMPOSE_SUCCEEDED="false"
  test_docker || return 0

  if (
    cd "$PROJECT_ROOT"
    HTTP_PORT="$HOST_PORT" \
    IMAGE_NAME="$IMAGE_NAME" \
    HTTP_BEARER_TOKEN="$BEARER_TOKEN" \
    MCP_BEARER_TOKEN="$BEARER_TOKEN" \
    MCP_ALLOWED_ORIGINS="$ALLOWED_ORIGINS" \
    DATABASE_URL="$DATABASE_URL" \
    docker compose -f "$COMPOSE_FILE" "$@"
  ); then
    LAST_COMPOSE_SUCCEEDED="true"
  else
    echo "docker compose falhou." >&2
  fi
}

build_stack() {
  write_title "Build da stack"
  invoke_compose build
}

start_stack() {
  write_title "Subir stack"
  ensure_bearer_token
  invoke_compose up -d --build
  if [[ "$LAST_COMPOSE_SUCCEEDED" == "true" ]]; then
    write_bearer_token_info
    write_database_url_info
  fi
}

stop_stack() {
  write_title "Parar stack"
  invoke_compose down
}

restart_stack() {
  stop_stack
  start_stack
}

recreate_stack() {
  write_title "Recriar stack"
  ensure_bearer_token
  invoke_compose up -d --build --force-recreate
  if [[ "$LAST_COMPOSE_SUCCEEDED" == "true" ]]; then
    write_bearer_token_info
    write_database_url_info
  fi
}

show_logs() {
  write_title "Logs"
  invoke_compose logs -f
}

show_status() {
  write_title "Status"
  invoke_compose ps
}

show_compose_config() {
  write_title "Config Docker Compose"
  invoke_compose config
}

require_curl() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl nao esta instalado ou nao esta no PATH." >&2
    return 1
  fi
}

test_health() {
  write_title "Health check"
  ensure_bearer_token
  require_curl || return 0
  if ! curl -sS "http://localhost:$HOST_PORT/health" \
    -H "Authorization: Bearer $BEARER_TOKEN" \
    -H "Accept: application/json, text/event-stream"; then
    echo "Falha ao chamar http://localhost:$HOST_PORT/health" >&2
  fi
  printf "\n"
}

test_scrape() {
  write_title "Teste POST /scrape"
  ensure_bearer_token
  require_curl || return 0
  if ! curl -sS -X POST "http://localhost:$HOST_PORT/scrape" \
    -H "Authorization: Bearer $BEARER_TOKEN" \
    -H "Accept: application/json, text/event-stream" \
    -H "Content-Type: application/json" \
    -d '{"searchQueries":["pizzarias em Curitiba"],"maxPlacesPerQuery":3,"scrapeEmails":false,"scrapePhones":false,"language":"pt-BR"}'; then
    echo "Falha ao chamar http://localhost:$HOST_PORT/scrape" >&2
  fi
  printf "\n"
}

test_mcp() {
  write_title "Teste POST /mcp"
  ensure_bearer_token
  require_curl || return 0
  if ! curl -sS -X POST "http://localhost:$HOST_PORT/mcp" \
    -H "Authorization: Bearer $BEARER_TOKEN" \
    -H "Accept: application/json, text/event-stream" \
    -H "MCP-Protocol-Version: 2025-06-18" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'; then
    echo "Falha ao chamar http://localhost:$HOST_PORT/mcp" >&2
  fi
  printf "\n"
}

show_menu() {
  clear || true
  write_felsen_banner
  printf "\n"
  echo "MCP Google Maps - Docker Dev"
  printf "\n"
  echo "Projeto:       $PROJECT_ROOT"
  echo "Compose:       $COMPOSE_FILE"
  echo "Imagem:        $IMAGE_NAME:latest"
  echo "Porta local:   $HOST_PORT"
  if [[ -n "${BEARER_TOKEN// }" ]]; then
    echo "Bearer token:  configurado"
  else
    echo "Bearer token:  sera gerado ao subir a stack"
  fi
  if [[ -n "${DATABASE_URL// }" ]]; then
    echo "Database URL:  configurado"
  else
    echo "Database URL:  nao configurado"
  fi
  echo "Token salvo:   $TOKEN_FILE"
  printf "\n"
  echo "1. Buildar stack"
  echo "2. Subir stack"
  echo "3. Parar stack"
  echo "4. Reiniciar stack"
  echo "5. Recriar stack"
  echo "6. Ver status"
  echo "7. Ver logs"
  echo "8. Health check"
  echo "9. Testar /scrape"
  echo "10. Testar /mcp"
  echo "11. Ver config compose"
  echo "0. Sair"
  printf "\n"
}

while true; do
  show_menu
  read -r -p "Escolha uma opcao: " choice

  case "$choice" in
    1) build_stack ;;
    2) start_stack ;;
    3) stop_stack ;;
    4) restart_stack ;;
    5) recreate_stack ;;
    6) show_status ;;
    7) show_logs ;;
    8) test_health ;;
    9) test_scrape ;;
    10) test_mcp ;;
    11) show_compose_config ;;
    0) break ;;
    *) echo "Opcao invalida." ;;
  esac

  if [[ "$choice" != "0" && "$choice" != "7" ]]; then
    printf "\n"
    read -r -p "Pressione Enter para continuar" _
  fi
done
