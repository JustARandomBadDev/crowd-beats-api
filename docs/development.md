# Développement, configuration et validation

## Prérequis et démarrage

Le module [go.mod](../go.mod) demande **Go 1.26.0** ; le Dockerfile et la CI utilisent Go 1.26. Docker et Docker Compose fournissent PostgreSQL 16. Avec une base PostgreSQL installée autrement, fournir `DATABASE_URL` vers cette base.

```bash
docker compose up -d --build
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready
```

Le Compose normal expose PostgreSQL sur `localhost:5432` et l'API sur `localhost:8080`. `docker compose down` les arrête en conservant le volume ; `docker compose down -v` effacerait ce volume. Pour lancer l'API avec Go sur l'hôte :

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/crowdbeats?sslmode=disable'
go run ./cmd/api
```

Ces credentials sont ceux du Compose de développement uniquement. Go ne charge pas `.env` directement ; le `Makefile` l'inclut, ou les variables peuvent être exportées par le shell. Ne commiter ni `.env`, ni URL DB réelle, ni secret Spotify, manager ou token de session.

## Variables d'environnement

| Variable                     | Requise           | Défaut du code / exemple                        | Usage                                                                      |
| ---------------------------- | ----------------- | ----------------------------------------------- | -------------------------------------------------------------------------- |
| `DATABASE_URL`               | Oui               | Aucun                                           | URL PostgreSQL ; erreur au chargement si absente.                          |
| `HTTP_ADDR`                  | Non               | `:8080`                                         | Adresse d'écoute HTTP.                                                     |
| `ALLOWED_ORIGINS`            | Non               | `*`                                             | Valeur du CORS HTTP.                                                       |
| `AUTO_MIGRATE`               | Non               | `true`                                          | Exécute `migrations/001_init.sql` au bootstrap.                            |
| `SHUTDOWN_TIMEOUT`           | Non               | `10s`                                           | Délai d'arrêt HTTP.                                                        |
| `SEARCH_CACHE_TTL`           | Non               | `30s`                                           | Durée du cache de recherche Spotify en mémoire.                            |
| `SPOTIFY_CLIENT_ID`          | Non               | Vide                                            | Client Credentials Spotify ; si absent, recherche dans le catalogue local. |
| `SPOTIFY_CLIENT_SECRET`      | Non               | Vide                                            | Secret Spotify ; même comportement si absent.                              |
| `SPOTIFY_TOKEN_URL`          | Non               | URL officielle du token Spotify                 | URL fournisseur, utile aux tests.                                          |
| `SPOTIFY_API_BASE`           | Non               | `https://api.spotify.com/v1`                    | Base API fournisseur, utile aux tests.                                     |
| `TEST_DATABASE_URL`          | Tests integration | Aucun dans le code ; exemple local port `55432` | Base dédiée pouvant être réinitialisée.                                    |
| `ALLOW_INTEGRATION_DB_RESET` | Tests integration | Valeur autre que `true` refusée                 | Accord explicite pour migrations et `TRUNCATE`.                            |

Les durées acceptent la syntaxe Go (`10s`, `1m`) ou un nombre entier de secondes. Le scheduler lit `rooms.recalc_interval_seconds` ; il n'existe plus de variable globale `DEFAULT_RECALC_INTERVAL`. Les secrets gérant et tokens de session sont générés par le serveur, retournés lors de leur acquisition, hashés en base et absents des autres réponses publiques. Les détails PostgreSQL et `raw_payload` Spotify ne sont pas exposés par l'API REST.

## Health checks

| État PostgreSQL |    `GET /health/live` |          `GET /health/ready` |
| --------------- | --------------------: | ---------------------------: |
| Disponible      | 200, `data.status=ok` |     200, `data.status=ready` |
| Indisponible    | 200, `data.status=ok` | 503, `data.status=not_ready` |

`ready` effectue un `Ping` PostgreSQL avec délai de 1,5 s. Spotify ne bloque pas la readiness. Avec `AUTO_MIGRATE=false`, le processus peut démarrer pendant une panne DB, mais `ready` reste à 503 jusqu'au retour de PostgreSQL.

## Tests Go

```bash
go build ./...
go test ./...
go vet ./...
go test -race ./...
git diff --check
```

Les tests HTTP et WebSocket légers sont inclus dans `go test ./...`. La CI lance `-race` sur `internal/infra/ws`, `internal/infra/scheduler` et `internal/usecase`, puis construit l'image Docker.

## Tests PostgreSQL isolés

Les tests avec tag `integration` appliquent la migration puis exécutent **`TRUNCATE ... RESTART IDENTITY CASCADE`**. Ils ne doivent jamais viser la base de développement normale, de staging ou de production. Le helper refuse l'absence de `TEST_DATABASE_URL`, l'absence de `ALLOW_INTEGRATION_DB_RESET=true`, une URL identique à `DATABASE_URL`, ou un nom de base sans `test`.

La configuration dédiée est [docker-compose.test.yml](../docker-compose.test.yml) : service `postgres-test`, PostgreSQL 16, base et utilisateur `crowdbeats_test`, port local `55432`, stockage éphémère et healthcheck. Exemple avec les **identifiants locaux de test** déjà présents dans [.env.example](../.env.example) :

```bash
docker compose -f docker-compose.test.yml up -d postgres-test
set -a
. ./.env.example
set +a
export ALLOW_INTEGRATION_DB_RESET=true
go test -count=1 -tags=integration ./...
docker compose -f docker-compose.test.yml down
```

Vérifier la cible de `TEST_DATABASE_URL` avant d'exporter le flag de reset. Le `Makefile` possède un raccourci historique `test-integration` dont le défaut de `TEST_DATABASE_URL` pointe sur `DATABASE_URL` ; utiliser la commande explicite ci-dessus. Le garde-fou du helper refusera cette cible si elle n'est pas dédiée. Les tests ne sont pas parallélisés entre eux car ils partagent une base réinitialisée ; certains tests lancent des goroutines pour vérifier la concurrence PostgreSQL.

## Docker et CI

```bash
docker build -t crowd-beats-api:local .
docker compose up -d --build
docker compose logs -f api postgres
docker compose down
```

Le Dockerfile construit le binaire Go, puis copie le binaire et les migrations dans une image Alpine avec certificats CA. Le workflow [CI](../.github/workflows/ci.yml) sur PR et push `main`/`dev` exécute build, tests unitaires, `vet`, tests ciblés `-race`, tests `integration` contre un service PostgreSQL 16 dédié, puis Docker build. Les tests CI n'utilisent pas de credentials Spotify réels. Le workflow de CD publie une image GHCR sur `main` ; il ne déploie pas directement l'API.
