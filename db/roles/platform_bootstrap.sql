-- Apply as DBA after creating a dedicated NOINHERIT LOGIN launlog_bootstrap.
-- Never give this role membership in launlog_owner or launlog_runtime.
GRANT CONNECT ON DATABASE :"database_name" TO launlog_bootstrap;
GRANT USAGE ON SCHEMA public TO launlog_bootstrap;
GRANT INSERT ON TABLE public.platform_admins TO launlog_bootstrap;
GRANT INSERT ON TABLE public.platform_audit_logs TO launlog_bootstrap;
GRANT USAGE ON SEQUENCE public.platform_audit_logs_id_seq TO launlog_bootstrap;
