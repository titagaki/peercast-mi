-- DESIGN DRAFT 2026-09-15. Not an application migration; not deployed.
-- Run only in a dedicated peercast_mi database after version/DDL validation.
-- All application timestamps are UTC. IDs are random 128-bit lowercase hex.

CREATE TABLE audit_events (
    event_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    node_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    boot_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    boot_seq BIGINT UNSIGNED NOT NULL,
    occurred_at DATETIME(6) NOT NULL,
    recorded_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    event_type VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    source VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    outcome VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    actor_account VARCHAR(255) COLLATE utf8mb4_bin NULL,
    actor_name VARCHAR(256) NULL,
    owner_account VARCHAR(255) COLLATE utf8mb4_bin NULL,
    client_ip VARCHAR(45) CHARACTER SET ascii COLLATE ascii_bin NULL,
    session_ref CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    broadcast_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    input_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    connection_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    channel_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    reason_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    payload_version SMALLINT UNSIGNED NOT NULL,
    payload JSON NOT NULL,
    PRIMARY KEY (event_id),
    UNIQUE KEY uq_event_order (node_id, boot_id, boot_seq),
    KEY ix_event_time (occurred_at, event_id),
    KEY ix_event_actor (actor_account, occurred_at, event_id),
    KEY ix_event_owner (owner_account, occurred_at, event_id),
    KEY ix_event_type (event_type, occurred_at, event_id),
    KEY ix_event_broadcast (broadcast_id, occurred_at, event_id),
    KEY ix_event_session (session_ref, occurred_at, event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE broadcasts (
    broadcast_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    node_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    boot_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    owner_account VARCHAR(255) COLLATE utf8mb4_bin NULL,
    owner_name VARCHAR(256) NULL,
    created_by VARCHAR(255) COLLATE utf8mb4_bin NULL,
    source VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    create_ip VARCHAR(45) CHARACTER SET ascii COLLATE ascii_bin NULL,
    channel_name VARCHAR(256) NOT NULL,
    input_genre VARCHAR(256) NULL,
    genre VARCHAR(256) NOT NULL,
    description TEXT NOT NULL,
    comment TEXT NOT NULL,
    contact_url TEXT NOT NULL,
    bitrate INT UNSIGNED NOT NULL,
    content_type VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    first_media_at DATETIME(6) NULL,
    last_media_at DATETIME(6) NULL,
    ended_at DATETIME(6) NULL,
    interruption_detected_at DATETIME(6) NULL,
    status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    end_reason VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT UNSIGNED NOT NULL,
    source_event_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (broadcast_id),
    KEY ix_broadcast_owner (owner_account, created_at, broadcast_id),
    KEY ix_broadcast_time (created_at, broadcast_id),
    KEY ix_broadcast_channel (channel_id, created_at, broadcast_id),
    KEY ix_broadcast_recovery (node_id, status, boot_id),
    KEY ix_broadcast_retention (status, ended_at, interruption_detected_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE broadcast_inputs (
    input_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    broadcast_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    connection_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    remote_ip VARCHAR(45) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    started_at DATETIME(6) NOT NULL,
    last_media_at DATETIME(6) NOT NULL,
    ended_at DATETIME(6) NULL,
    interruption_detected_at DATETIME(6) NULL,
    end_reason VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    incomplete BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT UNSIGNED NOT NULL,
    source_event_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (input_id),
    KEY ix_input_broadcast (broadcast_id, started_at, input_id),
    KEY ix_input_connection (connection_id, started_at, input_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Deployment tooling owns this table. Runtime credentials cannot perform DDL.
CREATE TABLE schema_migrations (
    version BIGINT UNSIGNED NOT NULL,
    checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
