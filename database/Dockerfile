FROM postgres:16-alpine

# Extensions on first start; the schema itself is migrated by the backend
# from /database/migrations (single source of truth).
COPY init/ /docker-entrypoint-initdb.d/
COPY postgresql.conf /etc/postgresql/postgresql.conf

CMD ["postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"]
