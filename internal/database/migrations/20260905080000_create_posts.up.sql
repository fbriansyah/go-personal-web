create table posts (
    id           uuid        primary key,
    slug         text        not null,
    title        text        not null,
    body         text        not null,
    published_at timestamptz,
    created_at   timestamptz not null,
    updated_at   timestamptz not null,

    -- A Slug is the only identifier a visitor sees, so its shape is enforced
    -- here rather than trusted to whatever writes the row.
    constraint posts_slug_format check (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    constraint posts_title_present check (length(btrim(title)) > 0)
);

create unique index posts_slug_key on posts (slug);

-- The feed reads `published_at is not null and published_at <= now()`; the
-- partial index carries only the rows that clause can return.
create index posts_published_at_idx on posts (published_at desc)
    where published_at is not null;
