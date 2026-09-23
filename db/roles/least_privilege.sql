-- Run as a database administrator after creating launlog_migrator and
-- launlog_runtime. Supply the actual database name with psql -v database_name=.
-- This file contains no passwords and grants no role/database administration.
GRANT CONNECT ON DATABASE :"database_name" TO launlog_migrator, launlog_runtime;
GRANT USAGE, CREATE ON SCHEMA public TO launlog_migrator;
GRANT USAGE ON SCHEMA public TO launlog_runtime;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO launlog_runtime;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO launlog_runtime;
REVOKE UPDATE, DELETE ON TABLE public.audit_logs FROM launlog_runtime;

ALTER DEFAULT PRIVILEGES FOR ROLE launlog_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO launlog_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE launlog_migrator IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO launlog_runtime;
