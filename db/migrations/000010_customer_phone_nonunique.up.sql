-- Business requirements do not require unique customer phone numbers.
-- Existing customer rows and order references are preserved.
DROP INDEX customers_business_phone_uq;
