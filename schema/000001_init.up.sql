-- CHECK constraints repeat the domain's invariants (see internal/*/domain) so
-- rows the application restores without re-validation are guaranteed valid.

CREATE TABLE users
(
    id            bigserial PRIMARY KEY,
    name          varchar(255) NOT NULL CHECK (btrim(name) <> ''),
    -- The application stores usernames lowercased; UNIQUE therefore also
    -- rejects case variants.
    username      varchar(255) NOT NULL UNIQUE CHECK (btrim(username) <> ''),
    password_hash varchar(255) NOT NULL CHECK (password_hash <> ''),
    version       bigint       NOT NULL DEFAULT 1 CHECK (version > 0) -- optimistic concurrency control
);

-- Cross-context link: the todo context owns lists by identity's user id, and
-- deleting a user deletes their lists.
CREATE TABLE todo_lists
(
    id          bigserial PRIMARY KEY,
    user_id     bigint       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title       varchar(255) NOT NULL CHECK (btrim(title) <> ''),
    description varchar(255) NOT NULL DEFAULT '',
    version     bigint       NOT NULL DEFAULT 1 CHECK (version > 0) -- optimistic concurrency control
);

CREATE INDEX todo_lists_user_id_idx ON todo_lists (user_id);

-- An item belongs to exactly one list and is deleted with it.
CREATE TABLE todo_items
(
    id          bigserial PRIMARY KEY,
    list_id     bigint       NOT NULL REFERENCES todo_lists (id) ON DELETE CASCADE,
    title       varchar(255) NOT NULL CHECK (btrim(title) <> ''),
    description varchar(255) NOT NULL DEFAULT '',
    done        boolean      NOT NULL DEFAULT false,
    version     bigint       NOT NULL DEFAULT 1 CHECK (version > 0) -- optimistic concurrency control
);

CREATE INDEX todo_items_list_id_idx ON todo_items (list_id);
