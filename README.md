# Crowd Beats API

Backend Go du MVP Crowd Beats.

Crowd Beats est une app type “Crowd DJ” :
- les utilisateurs rejoignent une room via QR code
- proposent des musiques Spotify
- votent pour influencer la queue
- reçoivent des mises à jour live via WebSocket
- le gérant crée les rooms, génère les QR codes, supprime ou skip des tracks et consulte les stats

Le projet suit une architecture de modular monolith documentée dans [docs/architecture.md](/home/chef/Dev/Crowd-Beats-API/docs/architecture.md), avec PostgreSQL comme source de vérité et une couche temps réel simple pour un MVP.

## Stack

- Go
- PostgreSQL
- WebSocket
- Docker / Docker Compose
- GitHub Actions
- GHCR pour la publication d’image

## Fonctionnalités principales

- API HTTP JSON versionnée sous `/api/v1`
- sessions anonymes persistées via bearer token opaque
- création de room et génération de QR code manager
- join room via QR code
- recherche Spotify via API Spotify ou fallback catalogue local
- proposition de track avec prévention des doublons actifs
- système de votes avec quota et contrainte d’unicité
- queue matérialisée en base, recalculée périodiquement
- WebSocket room-scoped pour les événements live

## Structure du projet

```text
cmd/api                point d’entrée binaire
internal/app           bootstrap et router
internal/domain        entités métier et interfaces repository
internal/usecase       orchestration métier
internal/infra         HTTP, PostgreSQL, WebSocket, scheduler, cache, client Spotify
internal/platform      utilitaires transverses
migrations             schéma PostgreSQL
pkg/apierror           erreurs API partagées
```

## Prérequis

- Go compatible avec `go.mod`
- Docker + Docker Compose
- PostgreSQL local si lancement sans Docker

## Configuration

Copier `.env.example` vers `.env` si tu veux piloter les commandes `make`.

Variables principales :

- `DATABASE_URL`
- `TEST_DATABASE_URL`
- `HTTP_ADDR`
- `AUTO_MIGRATE`
- `ALLOWED_ORIGINS`
- `DEFAULT_RECALC_INTERVAL`
- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`

Sans credentials Spotify, le backend reste exécutable mais la recherche Spotify ne renverra que les tracks déjà présentes dans le catalogue local.

## Lancement local sans Docker

```bash
cp .env.example .env
make dev
```

Ou manuellement :

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/crowdbeats?sslmode=disable
export AUTO_MIGRATE=true
go run ./cmd/api
```

## Lancement avec Docker Compose

```bash
cp .env.example .env
make up
```

Services exposés :

- API: `http://localhost:8080`
- PostgreSQL: `localhost:5432`

Arrêt :

```bash
make down
```

Logs :

```bash
make logs
```

## Commandes utiles

```bash
make build
make dev
make test
make test-unit
make test-integration
make fmt
make tidy
make docker-build
make up
make down
```

## Tests

Le repo contient trois niveaux de tests.

### Tests unitaires

Lancés par défaut avec :

```bash
make test-unit
```

Couvrent des cas ciblés :
- vote refusé si déjà voté
- proposition de track déjà active dans une room
- join par QR invalide

### Tests HTTP

Ils sont inclus dans `make test-unit` et utilisent `net/http/httptest`.

Cas couverts :
- health endpoint
- route protégée sans bearer token
- création de room via router/handler

### Tests d’intégration PostgreSQL

Ils sont taggés `integration` et utilisent une vraie base PostgreSQL.

```bash
make test-integration
```

Ils valident notamment :
- création / lecture d’une room
- création / lecture d’une session
- contrainte d’unicité des votes
- contrainte anti-doublon actif sur les tracks d’une room

Les tests d’intégration nécessitent `TEST_DATABASE_URL`. En local, le plus simple est de démarrer PostgreSQL via `make up` puis de lancer `make test-integration`.

## Docker

Le `Dockerfile` est multi-stage :
- build du binaire Go
- image runtime Alpine légère
- copie des migrations nécessaires au démarrage

Build local :

```bash
make docker-build
```

## CI/CD

### CI

Workflow : [.github/workflows/ci.yml](/home/chef/Dev/Crowd-Beats-API/.github/workflows/ci.yml)

À chaque PR / push :
- téléchargement des dépendances
- tests unitaires
- tests d’intégration avec PostgreSQL via `services`
- build Go
- build Docker

### CD

Workflow : [.github/workflows/cd.yml](/home/chef/Dev/Crowd-Beats-API/.github/workflows/cd.yml)

Sur `main` :
- build de l’image Docker
- push sur GHCR

Image publiée :

```text
ghcr.io/<owner>/crowd-beats-api:latest
ghcr.io/<owner>/crowd-beats-api:sha-...
```

## Déploiement MVP

Le chemin le plus simple pour un MVP :

1. utiliser l’image poussée sur GHCR
2. la brancher sur un provider simple type Render, Fly.io ou Railway
3. configurer les variables d’environnement suivantes :
   - `DATABASE_URL`
   - `AUTO_MIGRATE=true`
   - `HTTP_ADDR=:8080`
   - `ALLOWED_ORIGINS`
   - `SPOTIFY_CLIENT_ID`
   - `SPOTIFY_CLIENT_SECRET`

La CD fournie pousse l’image. Le déploiement final côté provider reste volontairement simple et découplé du repo pour éviter de sur-ingénier le MVP.

## Endpoints principaux

- `GET /health/live`
- `GET /health/ready`
- `POST /api/v1/manager/rooms`
- `POST /api/v1/manager/rooms/{roomId}/qr-codes`
- `POST /api/v1/rooms/join-by-qr`
- `GET /api/v1/spotify/search?q=...`
- `POST /api/v1/rooms/{roomId}/tracks`
- `POST /api/v1/rooms/{roomId}/votes`
- `GET /api/v1/rooms/{roomId}/queue`
- `GET /ws?room_id=<uuid>`

## Auth

- endpoints manager : header `X-Manager-Secret`
- endpoints user : `Authorization: Bearer <session_token>`

Le secret manager est renvoyé lors de la création d’une room. Le token utilisateur est renvoyé au join room via QR code.

## Roadmap technique courte

- compléter la couverture des usecases critiques
- enrichir les tests d’intégration concurrence
- brancher un vrai déploiement provider via secret ou hook GitHub Actions
- améliorer la readiness en branchant explicitement le check DB/Spotify si nécessaire
