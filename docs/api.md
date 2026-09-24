# Contrat REST `/api/v1`

Référence du client Flutter et du dashboard gérant. Sources du contrat : `internal/app/router.go`, `internal/infra/http/dto/public.go`, handlers et use cases. Tous les identifiants UUID sont des **chaînes JSON** ; les timestamps sont des chaînes RFC3339 (ou `null` si le DTO l'indique). Les clés publiques sont en `snake_case`.

## Enveloppe et authentification

Toute réponse JSON des handlers ci-dessous, health compris, utilise :

```json
{"data": {}, "error": null, "meta": {}}
```

En cas d'erreur :

```json
{"data": null, "error": {"code": "VALIDATION_ERROR", "message": "room name is required"}, "meta": {}}
```

Le client doit décider avec `error.code` et le statut HTTP, pas analyser `message`. Un body JSON illisible ou plusieurs valeurs JSON donnent `400 INVALID_JSON`. Un UUID de path invalide donne `400 INVALID_ID` ; un UUID valide absent donne `404 NOT_FOUND` ou un code métier indiqué plus bas. Un UUID invalide dans un body typé UUID donne `INVALID_JSON`.

**Session** : `Authorization: Bearer <session_token>`. Le token opaque de 64 caractères hexadécimaux est généré par le serveur et reçu au join ; `/sessions/me` ne le renvoie pas. **Manager** : `X-Manager-Secret: <manager_secret>`, remis lors de la création de la room. Ne pas journaliser ni commiter ces secrets ; leurs hashes ne figurent dans aucun DTO public.

### Codes d'erreur effectivement émis

| Statut | Code                                                                                            | Situation                                                                               |
| -----: | ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
|    400 | `INVALID_JSON`, `INVALID_ID`, `VALIDATION_ERROR`, `INVALID_SPOTIFY_TRACK_ID`                    | Body, UUID, champs ou ID Spotify invalides.                                             |
|    401 | `UNAUTHORIZED`                                                                                  | Bearer manquant, inconnu ou session inactive.                                           |
|    403 | `MANAGER_FORBIDDEN`, `INVALID_QR_CODE`, `ROOM_NOT_ACTIVE`, `SESSION_NOT_IN_ROOM`                | Secret gérant invalide, QR inutilisable, room inactive ou session hors room.            |
|    404 | `NOT_FOUND`, `ROOM_TRACK_NOT_FOUND`, `SPOTIFY_TRACK_NOT_FOUND`, parfois `ROOM_TRACK_NOT_ACTIVE` | Ressource absente.                                                                      |
|    409 | `QUEUE_FULL`, `ALREADY_VOTED_FOR_TRACK`, `VOTE_LIMIT_REACHED`, `ROOM_TRACK_NOT_ACTIVE`          | Conflit métier ; le dernier code correspond ici à une track existante mais non votable. |
|    500 | `INTERNAL_ERROR`                                                                                | Erreur inattendue ; le détail PostgreSQL n'est pas renvoyé.                             |
|    503 | `SPOTIFY_UNAVAILABLE`, `SPOTIFY_RATE_LIMITED`                                                   | Fournisseur indisponible ou rate limited.                                               |

`200` signifie succès, `201` création. Le doublon de proposition est un **succès 200**, pas un conflit 409. Les réponses avant upgrade WebSocket sont des erreurs HTTP simples ; voir [websocket.md](websocket.md).

## DTO réutilisés

| Objet                   | Champs JSON et types                                                                                                                                                                                                                                                                 |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `RoomResponse`          | `id: string`, `name: string`, `slug: string                                                                                                                                                                                                                                          | null`, `status: string`, `queue_limit: int`, `max_votes_per_user: int`, `qr_ttl_seconds: int`, `recalc_interval_seconds: int`, `created_at: string`, `updated_at: string`. |
| `SessionResponse`       | `id: string`, `room_id: string`, `nickname: string`, `role: string`, `status: string`, `last_seen_at: string`.                                                                                                                                                                       |
| `SpotifyTrackResponse`  | `spotify_track_id: string`, `title: string`, `artist_names: string`, `album_name: string`, `duration_ms: int`, `image_url: string`, `preview_url: string`, `uri: string`. `artist_names` est **une chaîne**, pas une liste ; les métadonnées absentes sont des chaînes vides ou `0`. |
| `QueueItemResponse`     | `position: int`, `room_track_id: string`, `score: int`, `vote_count: int`, `track: SpotifyTrackResponse`, `proposed_by: string                                                                                                                                                       | null`.                                                                                                                                                                     |
| `NowPlayingResponse`    | `room_track_id: string`, `vote_count: int`, `track: SpotifyTrackResponse`, `proposed_by: string                                                                                                                                                                                      | null` ; aucune position.                                                                                                                                                   |
| `QueueSnapshotResponse` | `items: QueueItemResponse[]`, `now_playing: NowPlayingResponse                                                                                                                                                                                                                       | null`, `updated_at: string                                                                                                                                                 | null`. Une liste vide est `[]`. |
| `QRCodeResponse`        | `code: string`, `expires_at: string`.                                                                                                                                                                                                                                                |
| `RoomStatsResponse`     | `active_users: int`, `tracks_in_queue: int`, `votes_count: int`, `top_tracks: [{title: string, artist_names: string, votes: int}]`.                                                                                                                                                  |

`fifo_order`, `raw_payload`, `manager_secret_hash` et `session_token_hash` restent internes. `updated_at` de la queue est `null` avant le premier batch. Les votes ou une action gérant peuvent précéder le prochain classement : `vote_received` signale un compteur récent, `queue_updated` livre le snapshot classé.

## Rooms et session

### `POST /api/v1/rooms/join-by-qr`

Auth : aucune. Body JSON : `qr_code` (chaîne), `nickname` (1–32 caractères après trim), `session_token` (chaîne facultative pour réutiliser une session connue). La room doit être `active` et le QR non expiré/non révoqué.

```json
{"qr_code":"qr_exemple","nickname":"Camille","session_token":""}
```

Réponse **200** : `data = {"room": RoomResponse, "session": {"id": string, "nickname": string, "role": string, "token": string}, "ws": {"url": string}}`. `ws.url` vaut un chemin relatif tel que `/ws?room_id=<uuid>` ; le WebSocket exige le bearer retourné. Erreurs : `400 INVALID_JSON/VALIDATION_ERROR`, `403 INVALID_QR_CODE/ROOM_NOT_ACTIVE`, `500 INTERNAL_ERROR`.

```json
{
  "data": {
    "room": {
      "id": "11111111-1111-4111-8111-111111111111",
      "name": "Le Bar",
      "slug": null,
      "status": "active",
      "queue_limit": 20,
      "max_votes_per_user": 5,
      "qr_ttl_seconds": 14400,
      "recalc_interval_seconds": 10,
      "created_at": "2026-09-21T08:00:00Z",
      "updated_at": "2026-09-21T08:00:00Z"
    },
    "session": {
      "id": "22222222-2222-4222-8222-222222222222",
      "nickname": "Camille",
      "role": "guest",
      "token": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    "ws": {"url": "/ws?room_id=11111111-1111-4111-8111-111111111111"}
  },
  "error": null,
  "meta": {}
}
```

Les valeurs ci-dessus sont fictives ; le token réel ne doit jamais figurer dans les logs du client.

| Token fourni         | Résultat                                                                                                     |
| -------------------- | ------------------------------------------------------------------------------------------------------------ |
| Aucun                | Nouvelle session et nouveau token serveur.                                                                   |
| Inconnu ou mal formé | Nouvelle session et nouveau token serveur ; la valeur fournie n'est pas conservée.                           |
| Connu, même room     | Même ID/token ; session réactivée, pseudo et présence mis à jour.                                            |
| Connu, autre room    | Ancienne session conservée dans son ancienne room et marquée `left` ; nouvel ID/token pour la nouvelle room. |

Un token ne contourne jamais la validation du QR ni du statut de room. Lors d'un changement de room, **remplacer le token local** par celui de la nouvelle réponse. La possession d'un ancien token ne constitue pas une route de reconnexion indépendante.

### `GET /api/v1/rooms/{roomID}`

Auth : aucune. Réponse **200** : `data = RoomResponse` (champs ci-dessus). Erreurs : `400 INVALID_ID`, `404 NOT_FOUND`.

### `GET /api/v1/sessions/me`

Auth : Bearer. Réponse **200** : `data = {"session": SessionResponse}`. Le token en clair et son hash sont absents. Erreur : `401 UNAUTHORIZED`.

### `POST /api/v1/sessions/heartbeat`

Auth : Bearer. Body JSON facultatif : `{}` ou `{"room_id":"<uuid de la session>"}` ; s'il est fourni, `room_id` vérifie le contexte et ne déplace jamais la session. Réponse **200** : `data = {"updated":true}`. Erreurs : `400 INVALID_JSON`, `401 UNAUTHORIZED`, `403 SESSION_NOT_IN_ROOM`.

Le heartbeat actualise `last_seen_at`. Une session `active` est comptée présente pendant **2 minutes** après sa dernière activité ; devenir stale ne supprime pas le token ni les votes. Recommandation client : heartbeat toutes les **30 à 60 s** tant que la room est ouverte. Ce rythme est une recommandation Flutter, pas une validation serveur.

### `POST /api/v1/sessions/leave`

Auth : Bearer, sans body requis. Réponse **200** : `data = {"left":true}`. La session passe à `left`, ses sockets WebSocket sont fermés et son historique reste en DB. Un nouveau join dans la même room avec le token encore connu peut la réactiver, si le QR et la room sont valides. Erreur : `401 UNAUTHORIZED` pour une session déjà inactive à l'authentification.

## Spotify, propositions et votes

### `GET /api/v1/spotify/search?q=<texte>`

Auth : Bearer. `q` vide ou composé d'espaces donne `400 VALIDATION_ERROR`. Réponse **200** : `data = {"items": SpotifyTrackResponse[]}` ; `items` est `[]` sans résultat. Avec les credentials Spotify, le backend interroge le fournisseur (cache de recherche court). Sans credentials, il recherche uniquement dans le catalogue local déjà stocké. Les échecs fournisseur donnent `503 SPOTIFY_UNAVAILABLE` ou `SPOTIFY_RATE_LIMITED`, sans corps fournisseur brut. Un champ `artist_names` est une chaîne et `raw_payload` n'est jamais public.

### `POST /api/v1/rooms/{roomID}/tracks`

Auth : Bearer actif dans cette room ; room `active`. Body :

```json
{"spotify_track_id":"0123456789ABCDEFGHIJKL"}
```

L'ID doit avoir **22 caractères alphanumériques** ; seul le serveur récupère les métadonnées depuis le catalogue local ou Spotify.

| Résultat                | Statut | `data`                                                                                            |
| ----------------------- | -----: | ------------------------------------------------------------------------------------------------- |
| Nouvelle proposition    |    201 | `{"room_track":{"id":"<uuid>","status":"queued","position":null},"duplicate":false}`              |
| Même morceau déjà actif |    200 | `{"existing_room_track":{"id":"<uuid>","current_vote_count":0,"position":null},"duplicate":true}` |

Par exemple, pour un doublon :

```json
{"data":{"existing_room_track":{"id":"11111111-1111-4111-8111-111111111111","current_vote_count":2,"position":1},"duplicate":true},"error":null,"meta":{}}
```

`room_track` et `existing_room_track` sont mutuellement exclusifs ; le champ absent est **omis** (pas `null`). `position` peut rester `null` avant le batch ou si la track joue. Les statuts `queued` **et** `playing` sont des doublons actifs. Une occurrence `played`/`skipped`, ou supprimée physiquement, peut être reproposée. Le doublon est testé **avant** `queue_limit` : une queue pleine n'empêche pas le retour du morceau déjà actif.

Erreurs : `400 INVALID_ID/INVALID_JSON/INVALID_SPOTIFY_TRACK_ID`, `401 UNAUTHORIZED`, `403 SESSION_NOT_IN_ROOM/ROOM_NOT_ACTIVE`, `404 NOT_FOUND/SPOTIFY_TRACK_NOT_FOUND`, `409 QUEUE_FULL`, `503 SPOTIFY_UNAVAILABLE/SPOTIFY_RATE_LIMITED`. Sans credentials et sans track dans le catalogue local, la récupération de cette track se traduit actuellement par `503 SPOTIFY_UNAVAILABLE` ; elle ne prouve pas que l'ID est absent chez Spotify.

### `POST /api/v1/rooms/{roomID}/votes`

Auth : Bearer actif dans la room `active`. Body :

```json
{"room_track_id":"11111111-1111-4111-8111-111111111111"}
```

Réponse **200** personnelle : `data = {"vote_added":true,"room_track_id":"<uuid>","current_vote_count":1,"votes_remaining":4}`. `votes_remaining` ne figure **jamais** dans le broadcast WebSocket. Une session vote au plus une fois par `room_track_id`, uniquement sur `queued` ou `playing`, et au plus `max_votes_per_user` votes dans la room. Une nouvelle occurrence du même Spotify ID a un nouvel ID et peut recevoir un nouveau vote.

Les votes des tracks `played`/`skipped` **restent dans le quota** tant que ces votes existent. Un hard delete supprime ses votes par cascade et libère donc du quota. Erreurs : `400 INVALID_ID/INVALID_JSON`, `401 UNAUTHORIZED`, `403 ROOM_NOT_ACTIVE/SESSION_NOT_IN_ROOM`, `404 ROOM_TRACK_NOT_ACTIVE` si ID absent, `409 ROOM_TRACK_NOT_ACTIVE/ALREADY_VOTED_FOR_TRACK/VOTE_LIMIT_REACHED` selon le cas.

## Queue

### `GET /api/v1/rooms/{roomID}/queue`

Auth : aucune. Une room inexistante donne **404 NOT_FOUND** ; une room existante sans morceau donne **200** avec `items: []` et `now_playing: null`. Réponse `data = QueueSnapshotResponse` :

La lecture reste possible pour une room `draft`, `paused` ou `closed` ; ces statuts bloquent les nouvelles actions client, pas ce GET.

```json
{
  "data": {
    "items": [{
      "position": 1,
      "room_track_id": "11111111-1111-4111-8111-111111111111",
      "score": 2,
      "vote_count": 2,
      "track": {
        "spotify_track_id": "0123456789ABCDEFGHIJKL",
        "title": "Exemple",
        "artist_names": "Artiste",
        "album_name": "Album",
        "duration_ms": 180000,
        "image_url": "",
        "preview_url": "",
        "uri": "spotify:track:0123456789ABCDEFGHIJKL"
      },
      "proposed_by": "Camille"
    }],
    "now_playing": null,
    "updated_at": "2026-09-21T08:00:00Z"
  },
  "error": null,
  "meta": {}
}
```

`items` contient uniquement les tracks `queued` classées par **score (= nombre de votes) décroissant**, puis `fifo_order` croissant en cas d'égalité. `fifo_order` reste interne. `queue_limit` limite le nombre de tracks `queued` ; `now_playing` est une track `playing` distincte, sans position et hors quota de queue. Son objet contient `room_track_id`, `vote_count`, `track` et `proposed_by`. Le batch utilise `recalc_interval_seconds` par room, défaut 10 s et plage 2–300 s. `updated_at` permet de comparer les versions batch des snapshots REST et `queue_updated` ; il peut être `null` avant le premier batch. Erreurs : `400 INVALID_ID`, `404 NOT_FOUND`.

## Manager

Le secret gérant est retourné **une seule fois** à la création de room. Toutes les routes gérant suivantes, sauf création, demandent `X-Manager-Secret` de la room du path. Une room absente donne `404 NOT_FOUND`, un mauvais secret `403 MANAGER_FORBIDDEN`.

### `POST /api/v1/manager/rooms`

Auth : aucune. Body : `name` requis (non vide après trim) ; `slug`, `status`, `queue_limit`, `max_votes_per_user`, `qr_ttl_seconds`, `recalc_interval_seconds` facultatifs. Exemple :

```json
{"name":"Le Bar","status":"draft","queue_limit":20,"max_votes_per_user":5,"qr_ttl_seconds":14400,"recalc_interval_seconds":10}
```

Défauts : `status=draft`, `queue_limit=20`, `max_votes_per_user=5`, `qr_ttl_seconds=14400`, `recalc_interval_seconds=10`. Bornes : 1–200, 1–20, 60–604800 et 2–300 respectivement. Réponse **201** : `data = {"room": RoomResponse, "manager_secret": string}`. Conserver le secret côté gérant ; seul son hash est stocké. Erreur : `400 VALIDATION_ERROR/INVALID_JSON` ; autres erreurs inattendues `500 INTERNAL_ERROR`.

### `POST /api/v1/manager/rooms/{roomID}/qr-codes`

Auth : manager. Body facultatif `{}` ou `{"expires_in_seconds":3600}`. Sans override, durée `room.qr_ttl_seconds` ; override autorisé de **60 à 604800 s**. Réponse **201** : `data = {"code":string,"expires_at":string}`. Une transaction sérialise la rotation et révoque les QR actifs précédents ; le join refuse les codes inconnus, expirés ou révoqués. Une room `draft` peut recevoir un QR, mais le join exige `active`. Erreurs : `400 INVALID_JSON/VALIDATION_ERROR/INVALID_ID`, `403 MANAGER_FORBIDDEN`, `404 NOT_FOUND`.

L'API renvoie le **code à transmettre/encoder**, pas une image QR PNG ni une URL de join. Le dashboard peut présenter ce code au client qui l'enverra comme `qr_code` au join.

### `PATCH /api/v1/manager/rooms/{roomID}`

Auth : manager. Body facultatif avec seulement `status`, `queue_limit`, `max_votes_per_user` ; le PATCH ne modifie actuellement ni la durée QR ni l'intervalle batch. Statuts admis : `draft`, `active`, `paused`, `closed`. Réponse **200** : `data = {"room": RoomResponse}`. Erreurs : `400 INVALID_JSON/VALIDATION_ERROR/INVALID_ID`, `403 MANAGER_FORBIDDEN`, `404 NOT_FOUND`.

| Statut   |           Join | Proposition | Vote |
| -------- | -------------: | ----------: | ---: |
| `draft`  |            non |         non |  non |
| `active` | oui, QR valide |         oui |  oui |
| `paused` |            non |         non |  non |
| `closed` |            non |         non |  non |

Un changement de statut ne supprime pas automatiquement sessions, tracks, votes ou queue.

### `GET /api/v1/manager/rooms/{roomID}/stats`

Auth : manager. Réponse **200** : `data = RoomStatsResponse` : `active_users` (présence <2 min), `tracks_in_queue` (`queued`), `votes_count` (votes DB), `top_tracks` (au plus cinq occurrences `queued`/`playing`, champs `title`, `artist_names`, `votes`). La route publique `/api/v1/rooms/{roomID}/stats` n'existe pas. Erreurs : `400 INVALID_ID`, `403 MANAGER_FORBIDDEN`, `404 NOT_FOUND`.

### Actions sur une occurrence de track

Toutes demandent le secret manager et renvoient une enveloppe 200. Elles écrivent leur mutation en DB, puis publient l'événement WebSocket correspondant après commit. Le prochain batch réordonne la file.

| Route                                                           | Effet                                                                                                                  | `data` 200                                                     | Erreurs métier              |
| --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | --------------------------- |
| `DELETE /api/v1/rooms/{roomID}/tracks/{roomTrackID}`            | Hard delete de l'occurrence ; votes et entrée de queue supprimés par cascade, quota potentiellement libéré.            | `{"deleted":true,"room_track_id":"<uuid>"}`                    | `404 ROOM_TRACK_NOT_FOUND`  |
| `POST /api/v1/rooms/{roomID}/tracks/{roomTrackID}/skip`         | `queued`/`playing` → `skipped` ; historique conservé.                                                                  | `{"updated":true,"room_track_id":"<uuid>","status":"skipped"}` | `404 ROOM_TRACK_NOT_ACTIVE` |
| `POST /api/v1/rooms/{roomID}/tracks/{roomTrackID}/mark-playing` | Ancienne `playing` → `queued`, cible `queued`/`playing` → `playing` ; au plus une `playing` via les flows applicatifs. | `{"updated":true,"room_track_id":"<uuid>","status":"playing"}` | `404 ROOM_TRACK_NOT_ACTIVE` |
| `POST /api/v1/rooms/{roomID}/tracks/{roomTrackID}/mark-played`  | `queued`/`playing` → `played`, hors queue et `now_playing` au snapshot suivant.                                        | `{"updated":true,"room_track_id":"<uuid>","status":"played"}`  | `404 ROOM_TRACK_NOT_ACTIVE` |

Pour toutes : `400 INVALID_ID` sur path invalide, `403 MANAGER_FORBIDDEN`, `404 NOT_FOUND` sur room absente. Les votes d'une track `skipped` ou `played` restent dans le quota.

## Parcours recommandé pour Flutter

1. Scanner le QR, demander un pseudo, appeler `join-by-qr`, enregistrer `session.token` localement et remplacer l'ancien token si la room change.
2. Ouvrir `ws.url` avec le bearer, attendre `sync_required`, puis appeler `GET /queue` et afficher le snapshot. Voir [protocole WebSocket](websocket.md).
3. Envoyer un heartbeat toutes les 30–60 s lorsque la room est ouverte. Un redémarrage de l'app peut relire son token local et le fournir à `join-by-qr` avec un QR encore valide ; aucune route de reconnexion sans QR n'est prévue.
4. Rechercher Spotify, puis proposer seulement `spotify_track_id`. Sur **201**, attendre le classement batch. Sur **200 `duplicate=true`**, utiliser `existing_room_track.id` pour proposer un vote plutôt qu'une seconde proposition.
5. Après `POST /votes`, mettre à jour le **quota personnel** avec `votes_remaining` de REST. `vote_received` met à jour le compteur visible des autres clients ; le nouvel ordre vient plus tard via `queue_updated`.
6. À toute fermeture du socket ou redémarrage serveur : reconnecter, attendre `sync_required`, refaire `GET /queue`. Ne reconstruire ni la file ni la présence à partir d'un historique d'événements perdu.
