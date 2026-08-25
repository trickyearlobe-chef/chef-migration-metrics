-- Migration 0067: a credential a person makes for a tool they are holding.
--
-- WHY: an editor assistant needs one address and one credential. Until now the
-- only credential this service had was a session, which is short-lived, made by
-- a login screen, and indistinguishable from a browser once it is in a header.
-- That means a tool either got somebody's password or got nothing.
--
-- A credential is another way into the SAME account. There is no service
-- account and no second permissions model: the role comes from the account the
-- credential belongs to, read at every request, so a demotion applies to the
-- credential the moment it applies to the person.
--
-- Only the hash is stored. The secret is returned once, at creation, and there
-- is nowhere in this table it could be recovered from — which is the point, not
-- an inconvenience: the reason to destroy one is believing somebody else has
-- it, and a copy we could hand back is a copy somebody else could take.
--
-- can_write is chosen by whoever is about to hand the credential to a tool,
-- because only they know what it is for. FALSE is the default, in the column as
-- well as in the handler, so a credential made by something that never heard of
-- the flag cannot write.

CREATE TABLE api_tokens (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The account this is another way into. Not a foreign key, matching
    -- sessions: a person authenticated by an identity provider need not have a
    -- local row, and refusing them a credential would exclude every such person.
    username     TEXT        NOT NULL,

    -- What its owner called it, so a listing can be read and one of them
    -- destroyed without guessing. Also what an entry it writes is signed with.
    name         TEXT        NOT NULL,

    -- SHA-256 of the secret, hex. Unique so a lookup is by hash rather than by
    -- scanning, and so a duplicate is a collision rather than two live rows.
    token_hash   TEXT        NOT NULL,

    can_write    BOOLEAN     NOT NULL DEFAULT FALSE,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Roughly when, not exactly: written at most once a minute by the
    -- authentication path, because the alternative is a write on every request
    -- an assistant makes and the question being answered is only "is this one
    -- still in use". NULL until first use.
    last_used_at TIMESTAMPTZ,

    CONSTRAINT uq_api_tokens_hash UNIQUE (token_hash),
    CONSTRAINT uq_api_tokens_owner_name UNIQUE (username, name),
    CONSTRAINT chk_api_tokens_name_not_blank CHECK (btrim(name) <> '')
);

CREATE INDEX idx_api_tokens_username ON api_tokens (username);
