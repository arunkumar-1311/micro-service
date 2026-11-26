#!/bin/sh
# Creates the payment database (each service owns its own DB) and applies its migration.
set -e
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres -c "CREATE DATABASE payment"
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d payment -f /migrations/payment/001_init.sql
