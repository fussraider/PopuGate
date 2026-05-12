-- Add tags column to secret_templates.
ALTER TABLE secret_templates ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
