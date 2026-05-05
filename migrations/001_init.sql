-- init migration sourced from docs/schema.sql
begin;

create extension if not exists pgcrypto;

create or replace function set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

create sequence if not exists room_track_fifo_seq as bigint;

create table if not exists rooms (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  slug text unique,
  status text not null
    check (status in ('draft', 'active', 'paused', 'closed')),
  queue_limit integer not null default 20
    check (queue_limit > 0 and queue_limit <= 200),
  max_votes_per_user integer not null default 5
    check (max_votes_per_user between 1 and 20),
  qr_ttl_seconds integer not null default 14400
    check (qr_ttl_seconds between 60 and 604800),
  recalc_interval_seconds integer not null default 10
    check (recalc_interval_seconds between 2 and 300),
  manager_secret_hash text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_rooms_status on rooms(status);

create table if not exists room_qr_codes (
  id uuid primary key default gen_random_uuid(),
  room_id uuid not null references rooms(id) on delete cascade,
  code text not null unique,
  expires_at timestamptz not null,
  revoked boolean not null default false,
  created_at timestamptz not null default now(),
  check (char_length(code) >= 8)
);

create index if not exists idx_room_qr_codes_room_id on room_qr_codes(room_id);
create index if not exists idx_room_qr_codes_expires_at on room_qr_codes(expires_at);
create index if not exists idx_room_qr_codes_room_revoked_expires
  on room_qr_codes(room_id, revoked, expires_at);

create table if not exists user_sessions (
  id uuid primary key default gen_random_uuid(),
  room_id uuid not null references rooms(id) on delete cascade,
  session_token_hash text not null unique,
  nickname text not null
    check (char_length(trim(nickname)) between 1 and 32),
  role text not null
    check (role in ('guest', 'manager')),
  status text not null default 'active'
    check (status in ('active', 'left', 'expired', 'banned')),
  joined_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  left_at timestamptz,
  client_fingerprint text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (left_at is null or left_at >= joined_at)
);

create index if not exists idx_user_sessions_room_id_status
  on user_sessions(room_id, status);
create index if not exists idx_user_sessions_last_seen_at
  on user_sessions(last_seen_at);
create index if not exists idx_user_sessions_room_role_status
  on user_sessions(room_id, role, status);

create table if not exists spotify_tracks (
  id uuid primary key default gen_random_uuid(),
  spotify_track_id text not null unique,
  title text not null,
  artist_names text not null,
  album_name text,
  duration_ms integer check (duration_ms is null or duration_ms > 0),
  image_url text,
  preview_url text,
  uri text,
  raw_payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_spotify_tracks_title on spotify_tracks(title);

create table if not exists room_tracks (
  id uuid primary key default gen_random_uuid(),
  room_id uuid not null references rooms(id) on delete cascade,
  spotify_track_ref_id uuid not null references spotify_tracks(id) on delete restrict,
  proposed_by_session_id uuid references user_sessions(id) on delete set null,
  status text not null default 'queued'
    check (status in ('queued', 'playing', 'played', 'skipped', 'deleted')),
  vote_count_cached integer not null default 0 check (vote_count_cached >= 0),
  score_cached integer not null default 0,
  fifo_order bigint not null default nextval('room_track_fifo_seq'),
  proposed_at timestamptz not null default now(),
  played_at timestamptz,
  skipped_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (
    (status <> 'played' or played_at is not null)
    and (status <> 'skipped' or skipped_at is not null)
    and (status <> 'deleted' or deleted_at is not null)
  )
);

create index if not exists idx_room_tracks_room_status
  on room_tracks(room_id, status);
create index if not exists idx_room_tracks_room_fifo
  on room_tracks(room_id, fifo_order);
create index if not exists idx_room_tracks_proposed_by
  on room_tracks(proposed_by_session_id);
create index if not exists idx_room_tracks_room_created_at
  on room_tracks(room_id, created_at desc);

create unique index if not exists uq_room_tracks_active_unique_track
  on room_tracks(room_id, spotify_track_ref_id)
  where status in ('queued', 'playing');

create table if not exists votes (
  id uuid primary key default gen_random_uuid(),
  room_id uuid not null references rooms(id) on delete cascade,
  room_track_id uuid not null references room_tracks(id) on delete cascade,
  session_id uuid not null references user_sessions(id) on delete cascade,
  weight smallint not null default 1 check (weight = 1),
  created_at timestamptz not null default now()
);

create unique index if not exists uq_votes_session_track
  on votes(session_id, room_track_id);
create index if not exists idx_votes_room_track_id
  on votes(room_track_id);
create index if not exists idx_votes_session_id
  on votes(session_id);
create index if not exists idx_votes_room_id
  on votes(room_id);
create index if not exists idx_votes_room_session
  on votes(room_id, session_id);

create table if not exists room_queue (
  room_id uuid not null references rooms(id) on delete cascade,
  room_track_id uuid not null references room_tracks(id) on delete cascade,
  position integer not null check (position > 0),
  score integer not null,
  vote_count integer not null check (vote_count >= 0),
  fifo_order bigint not null,
  recalculated_at timestamptz not null,
  primary key (room_id, room_track_id)
);

create unique index if not exists uq_room_queue_room_position
  on room_queue(room_id, position);
create index if not exists idx_room_queue_room_position
  on room_queue(room_id, position);
create index if not exists idx_room_queue_room_recalculated_at
  on room_queue(room_id, recalculated_at desc);

create table if not exists room_events (
  id uuid primary key default gen_random_uuid(),
  room_id uuid not null references rooms(id) on delete cascade,
  event_type text not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_room_events_room_created_at
  on room_events(room_id, created_at desc);
create index if not exists idx_room_events_event_type
  on room_events(event_type);

create or replace function trg_room_tracks_validate_proposer_room()
returns trigger
language plpgsql
as $$
declare
  proposer_room_id uuid;
begin
  if new.proposed_by_session_id is null then
    return new;
  end if;

  select room_id into proposer_room_id
  from user_sessions
  where id = new.proposed_by_session_id;

  if proposer_room_id is null then
    raise exception 'proposed_by_session_id % does not exist', new.proposed_by_session_id;
  end if;

  if proposer_room_id <> new.room_id then
    raise exception 'proposer session room mismatch';
  end if;

  return new;
end;
$$;

drop trigger if exists room_tracks_validate_proposer_room on room_tracks;
create trigger room_tracks_validate_proposer_room
before insert or update of room_id, proposed_by_session_id
on room_tracks
for each row
execute function trg_room_tracks_validate_proposer_room();

create or replace function trg_votes_validate_room_consistency()
returns trigger
language plpgsql
as $$
declare
  v_session_room_id uuid;
  v_track_room_id uuid;
begin
  select room_id into v_session_room_id from user_sessions where id = new.session_id;
  if v_session_room_id is null then
    raise exception 'session_id % does not exist', new.session_id;
  end if;

  select room_id into v_track_room_id from room_tracks where id = new.room_track_id;
  if v_track_room_id is null then
    raise exception 'room_track_id % does not exist', new.room_track_id;
  end if;

  if new.room_id <> v_session_room_id or new.room_id <> v_track_room_id then
    raise exception 'vote room mismatch between vote, session and track';
  end if;
  return new;
end;
$$;

drop trigger if exists votes_validate_room_consistency on votes;
create trigger votes_validate_room_consistency
before insert or update of room_id, room_track_id, session_id
on votes
for each row
execute function trg_votes_validate_room_consistency();

create or replace function trg_votes_refresh_room_track_cache()
returns trigger
language plpgsql
as $$
begin
  if tg_op = 'INSERT' then
    update room_tracks
    set vote_count_cached = vote_count_cached + 1,
        updated_at = now()
    where id = new.room_track_id;
    return new;
  elsif tg_op = 'DELETE' then
    update room_tracks
    set vote_count_cached = greatest(vote_count_cached - 1, 0),
        updated_at = now()
    where id = old.room_track_id;
    return old;
  end if;
  return null;
end;
$$;

drop trigger if exists votes_after_insert_refresh_room_track_cache on votes;
create trigger votes_after_insert_refresh_room_track_cache
after insert on votes
for each row
execute function trg_votes_refresh_room_track_cache();

drop trigger if exists votes_after_delete_refresh_room_track_cache on votes;
create trigger votes_after_delete_refresh_room_track_cache
after delete on votes
for each row
execute function trg_votes_refresh_room_track_cache();

drop trigger if exists rooms_set_updated_at on rooms;
create trigger rooms_set_updated_at before update on rooms for each row execute function set_updated_at();
drop trigger if exists user_sessions_set_updated_at on user_sessions;
create trigger user_sessions_set_updated_at before update on user_sessions for each row execute function set_updated_at();
drop trigger if exists spotify_tracks_set_updated_at on spotify_tracks;
create trigger spotify_tracks_set_updated_at before update on spotify_tracks for each row execute function set_updated_at();
drop trigger if exists room_tracks_set_updated_at on room_tracks;
create trigger room_tracks_set_updated_at before update on room_tracks for each row execute function set_updated_at();

commit;
