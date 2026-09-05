create table pages (
    id           uuid        primary key,
    slug         text        not null,
    title        text        not null,
    body         text        not null,
    published_at timestamptz,
    created_at   timestamptz not null,
    updated_at   timestamptz not null,

    constraint pages_slug_format check (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    constraint pages_title_present check (length(btrim(title)) > 0)
);

-- A Page is only ever addressed by its Slug, so there is no chronological
-- index to serve here.
create unique index pages_slug_key on pages (slug);
