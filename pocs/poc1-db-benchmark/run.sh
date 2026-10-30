#!/usr/bin/env bash
# PoC-1 orchestrator: (re)create the database, then for each scale apply the
# schema (which wipes the tables), generate a tree, and benchmark it.
#
# Env overrides: PGBIN, PGHOST, PGPORT, PGUSER, DBNAME, SAMPLES, SCALES, SEED.
set -euo pipefail

PGBIN="${PGBIN:-/usr/lib/postgresql/16/bin}"
PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5433}"
PGUSER="${PGUSER:-postgres}"
DBNAME="${DBNAME:-familygraph_poc}"
SAMPLES="${SAMPLES:-400}"
SEED="${SEED:-42}"
SCALES="${SCALES:-1000 10000 50000}"
DSN="host=${PGHOST} port=${PGPORT} dbname=${DBNAME} user=${PGUSER}"
HERE="$(cd "$(dirname "$0")" && pwd)"

echo "# (re)creating database ${DBNAME}"
"${PGBIN}/dropdb"   -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" --if-exists "$DBNAME"
"${PGBIN}/createdb" -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" "$DBNAME"

for N in $SCALES; do
    echo
    echo "############ scale ${N} ############"
    "${PGBIN}/psql" "$DSN" -v ON_ERROR_STOP=1 -q -f "${HERE}/sql/01_schema.sql"
    python3 "${HERE}/generate.py"  --n "$N" --seed "$SEED" --dsn "$DSN"
    python3 "${HERE}/benchmark.py" --samples "$SAMPLES" --dsn "$DSN" \
        | tee "${HERE}/results/scale_${N}.txt"
done

echo
echo "# done. per-scale results in ${HERE}/results/"
