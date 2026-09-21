# Crowd Beats

Backend Go du MVP « Crowd DJ » pour bars et clubs. Un client scanne un QR temporaire, rejoint une room sous pseudo, recherche des morceaux Spotify, propose et vote. Le gérant crée la room, renouvelle son QR, consulte la file et les statistiques, puis pilote manuellement son système audio. **Le backend ne lance pas la lecture Spotify.** Flutter est le client mobile prévu ; il ne se trouve pas dans ce dépôt.

Le MVP comprend des sessions anonymes persistantes, une file recalculée par batch et des événements WebSocket. Il n'inclut ni comptes utilisateur, ni IA, recommandations, gamification, lecture automatique, fournisseur musical supplémentaire ou broadcast WebSocket entre plusieurs instances API.

## Stack et fonctionnement

Go 1.26, PostgreSQL 16 dans les configurations Docker, API HTTP JSON, WebSocket, Spotify API, Docker Compose et GitHub Actions. Les commandes passent par `handler → use case → repository → PostgreSQL`. La base est la source de vérité ; le registre WebSocket est en mémoire et suppose une instance API pour le temps réel.

## Démarrage rapide

Avec Docker et Docker Compose :

```bash
docker compose up -d --build
curl http://localhost:8080/health/ready
```

Compose démarre PostgreSQL et l'API, applique `migrations/001_init.sql` au démarrage (`AUTO_MIGRATE=true`) et expose l'API sur `localhost:8080`. `docker compose down` arrête les services ; le volume PostgreSQL est conservé. Les identifiants du Compose sont **locaux uniquement**.

Pour lancer Go sur l'hôte avec le PostgreSQL du Compose :

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/crowdbeats?sslmode=disable'
go run ./cmd/api
```

Go ne charge pas automatiquement `.env` : exporter les variables, ou utiliser le `Makefile` qui inclut ce fichier. Voir [configuration et tests](docs/development.md).

## Contrat et documentation

- [API REST et parcours Flutter](docs/api.md)
- [Protocole WebSocket et reconnexion](docs/websocket.md)
- [Architecture, transactions et scheduler](docs/architecture.md)
- [Schéma PostgreSQL](docs/database.md)
- [Développement, Docker, configuration et CI](docs/development.md)

L'API métier est sous `/api/v1`. Le join remet un bearer token de session ; la création d'une room remet un secret gérant. Conserver ces secrets localement et ne jamais les commiter. Les réponses publiques utilisent une enveloppe `data/error/meta` et des champs `snake_case`.

## Vérification rapide

```bash
go build ./...
go test ./...
go vet ./...
go test -race ./...
docker build -t crowd-beats-api:local .
```

Les tests PostgreSQL réinitialisent une **base dédiée**. Leur procédure et les deux garde-fous obligatoires sont dans [docs/development.md](docs/development.md). La CI exécute aussi ces tests et le build Docker. Le workflow de CD publie une image GHCR ; il ne déploie pas de fournisseur d'hébergement.

## Limites du MVP

Le client récupère l'état après une perte WebSocket via `sync_required` puis `GET /queue` : il n'y a pas de replay. Le classement de la file suit les votes puis l'ordre FIFO lors du batch, pas à chaque vote. Le gérant reste responsable de la lecture réelle. Le document [roadmap.md](docs/roadmap.md) est un plan historique, pas le contrat de l'API actuelle.
