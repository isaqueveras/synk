# Running Synk with PostgreSQL using Docker Compose

This example demonstrates how to run the Synk job processor with a PostgreSQL database using Docker Compose.

## Prerequisites
- [Docker](https://www.docker.com/get-started)
- [Docker Compose](https://docs.docker.com/compose/)

## How it works
- The `docker-compose.yml` file will start:
  - A PostgreSQL database (with schema and tables initialized from `init.sql`)
  - A `producer` service (Go app that produces jobs)
  - A `consumer` service (Go app that consumes/processes jobs)

## Usage

1. **Clone the repository** (if you haven't already):
   ```sh
   git clone https://github.com/isaqueveras/synk.git
   cd synk/example/postgres
   ```

2. **Start all services with Docker Compose:**
   ```sh
   docker compose up
   ```
   This will build and start the PostgreSQL, producer, and consumer containers.

3. **Check logs:**
   - Producer logs: `docker logs pg_synk_app_producer`
   - Consumer logs: `docker logs pg_synk_app_consumer`
   - Database logs: `docker logs pg_synk_db_postgres`

4. **Stop all services:**
   ```sh
   docker compose down
   ```
   To remove all data and re-initialize the database, add the `-v` flag:
   ```sh
   docker compose down -v
   ```

## Notes
- The database schema is initialized from `init.sql` on the first run (when the volume is empty).
- If you change `init.sql` and want to re-apply it, you must remove the volume with `docker compose down -v`.
- The environment variable `SYNK_DATABASE_POSTGRES` is used to configure the database connection string for the Go services.

---

For more details, see the main project documentation.
