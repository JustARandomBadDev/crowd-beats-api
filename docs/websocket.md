# Protocole WebSocket

Endpoint : `GET /ws?room_id=<uuid>` avec `Authorization: Bearer <session_token>`. Le token provient de `POST /api/v1/rooms/join-by-qr`. Avant l'upgrade, le serveur vérifie que la session existe, est `active` et appartient au `room_id` demandé ; il revérifie la session après enregistrement pour couvrir un switch concurrent. Un bearer invalide n'ouvre pas de socket. Les erreurs de handshake sont des réponses HTTP simples (401 pour une session invalide, 400 pour un `room_id` invalide, 403 pour une autre room, 500 pour une erreur d'authentification interne), **sans enveloppe REST**. Le corps d'une erreur d'authentification reste générique et n'expose aucun détail de la base de données.

Le WebSocket transporte principalement des messages **serveur → client**. Les commandes métier restent des requêtes REST. Le registre et ses hubs par room vivent en mémoire dans un seul processus API ; il n'y a ni replay, ni abonnement dynamique, ni broadcast entre replicas.

## Connexion et récupération de l'état

```text
1. POST join-by-qr et conserver session.token
2. ouvrir le socket avec ce bearer et le room_id
3. recevoir sync_required sur ce socket
4. GET /api/v1/rooms/{roomID}/queue
5. afficher ce snapshot REST, puis traiter les événements reçus
```

`sync_required` indique seulement que la connexion est prête : son payload est `{}`. Le serveur ne transmet pas le snapshot initial dans cet événement. Après perte réseau, fermeture de socket ou redémarrage backend, répéter **connexion → sync_required → GET queue**. `room_events` n'est pas un replay client. Si des événements arrivent pendant le GET, conserver le snapshot de queue le plus récent selon son `updated_at` lorsque les deux versions sont renseignées. `queue_updated` transporte toujours un snapshot complet, pas un delta. Une action gérant ou un vote peut être visible avant le prochain batch ; `updated_at` versionne le dernier snapshot classé et peut être `null` avant ce batch.

L'ouverture ou la reconnexion WebSocket ne produit pas `room_joined`. Cet événement suit un join métier REST réussi, y compris un rejoin via QR. `leave` et le switch vers une autre room ferment toutes les connexions de l'ancienne session ; le nouveau token remis au switch doit servir aux sockets suivants.

## Enveloppe et catalogue

Chaque événement est un objet JSON :

```json
{
  "event": "sync_required",
  "room_id": "11111111-1111-4111-8111-111111111111",
  "timestamp": "2026-09-21T08:00:00Z",
  "payload": {}
}
```

`timestamp` est l'heure serveur de l'événement. Les UUID sont des chaînes et les clés du payload sont en `snake_case`.

| Event              | Déclencheur                                          | Scope                                             | Payload exact                                                                                        |
| ------------------ | ---------------------------------------------------- | ------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `sync_required`    | Connexion authentifiée et activée.                   | Ce socket uniquement.                             | `{}`                                                                                                 |
| `room_joined`      | `POST join-by-qr` réussi, après commit.              | Room rejointe.                                    | `session_id: string`, `nickname: string`.                                                            |
| `presence_updated` | Join, leave ou switch de room, après mutation.       | Room concernée ; les deux rooms lors d'un switch. | `active_users: int`.                                                                                 |
| `track_proposed`   | Nouvelle proposition créée, après commit.            | Room.                                             | `room_track_id: string`, `spotify_track_id: string`, `title: string`, `artist_names: string`.        |
| `vote_received`    | Vote ajouté, après commit.                           | Room.                                             | `room_track_id: string`, `current_vote_count: int`.                                                  |
| `queue_updated`    | Batch ayant changé le snapshot public, après commit. | Room.                                             | **`QueueSnapshotResponse` exact** : `items`, `now_playing`, `updated_at` ; voir [API](api.md#queue). |
| `track_deleted`    | Hard delete, après commit.                           | Room.                                             | `room_track_id: string`.                                                                             |
| `track_skipped`    | Passage à `skipped`, après commit.                   | Room.                                             | `room_track_id: string`.                                                                             |
| `track_playing`    | Passage à `playing`, après commit.                   | Room.                                             | `room_track_id: string`.                                                                             |
| `track_played`     | Passage à `played`, après commit.                    | Room.                                             | `room_track_id: string`.                                                                             |

Exemple `vote_received` :

```json
{
  "event": "vote_received",
  "room_id": "11111111-1111-4111-8111-111111111111",
  "timestamp": "2026-09-21T08:01:00Z",
  "payload": {"room_track_id":"22222222-2222-4222-8222-222222222222","current_vote_count":5}
}
```

`votes_remaining` est personnel et figure **uniquement dans la réponse REST** au votant. `track_proposed` confirme l'acceptation, pas une position définitive ; le classement vient du prochain `queue_updated`. Le score de queue reste le nombre de votes puis FIFO à égalité. Le passage des deux minutes de fenêtre de présence sans requête ne déclenche pas à lui seul un broadcast immédiat ; les statistiques REST calculent la présence récente lors de leur lecture.

## Vie des connexions

Le serveur envoie un ping environ toutes les **30 s**, attend un pong au plus **60 s**, impose une échéance d'écriture d'environ **10 s** et limite les messages entrants à **1024 octets**. Le client n'a pas de commande métier à envoyer sur le socket ; il doit répondre aux ping selon sa bibliothèque WebSocket. Un client lent dont le buffer sortant est plein est fermé, puis peut reconnecter et recharger REST. Plusieurs sockets de la même session sont tous fermés après `leave` ou switch de room.

Le broadcast est un effet externe après commit : une panne de diffusion ne rollback pas PostgreSQL. Le snapshot REST est donc indispensable pour retrouver l'état après un événement perdu.
