CREATE TABLE users
(
    id            bigserial PRIMARY KEY,
    name          varchar(255) NOT NULL,
    username      varchar(255) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL
);

-- A list has exactly one owner.
CREATE TABLE todo_lists
(
    id          bigserial PRIMARY KEY,
    user_id     bigint       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title       varchar(255) NOT NULL,
    description varchar(255) NOT NULL DEFAULT '',
    version     bigint       NOT NULL DEFAULT 1 -- optimistic concurrency control
);

CREATE INDEX todo_lists_user_id_idx ON todo_lists (user_id);

-- An item belongs to exactly one list and is deleted with it.
CREATE TABLE todo_items
(
    id          bigserial PRIMARY KEY,
    list_id     bigint       NOT NULL REFERENCES todo_lists (id) ON DELETE CASCADE,
    title       varchar(255) NOT NULL,
    description varchar(255) NOT NULL DEFAULT '',
    done        boolean      NOT NULL DEFAULT false,
    version     bigint       NOT NULL DEFAULT 1 -- optimistic concurrency control
);

CREATE INDEX todo_items_list_id_idx ON todo_items (list_id);
