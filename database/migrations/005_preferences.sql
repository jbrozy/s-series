-- Personal settings (theme, density, start page, shortcuts, navigation).
-- Stored per account so they follow the user across workstations.
ALTER TABLE users ADD COLUMN IF NOT EXISTS preferences JSONB NOT NULL DEFAULT '{}';
