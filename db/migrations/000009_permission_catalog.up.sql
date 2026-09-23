INSERT INTO permissions (code, description) VALUES
    ('CUSTOMERS_READ', 'View and search customers'),
    ('CUSTOMERS_WRITE', 'Create and update customers'),
    ('SERVICES_READ', 'View laundry services'),
    ('SERVICES_WRITE', 'Create, update, and deactivate laundry services'),
    ('PERFUMES_READ', 'View perfumes'),
    ('PERFUMES_WRITE', 'Create, update, and deactivate perfumes'),
    ('ORDERS_READ', 'View orders'),
    ('ORDERS_CREATE', 'Create orders'),
    ('ORDERS_UPDATE', 'Update order details and status'),
    ('ORDERS_CANCEL', 'Cancel orders'),
    ('PAYMENTS_READ', 'View order payments'),
    ('PAYMENTS_RECORD', 'Record order payments'),
    ('EXPENSES_READ', 'View expenses and categories'),
    ('EXPENSES_WRITE', 'Create and update expenses and categories'),
    ('REPORTS_READ', 'View business and outlet reports'),
    ('RECEIPTS_MANAGE', 'Manage receipt templates and settings'),
    ('AUDIT_READ', 'View audit history')
ON CONFLICT (code) DO NOTHING;
