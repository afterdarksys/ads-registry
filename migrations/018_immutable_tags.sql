-- Migration: 018_immutable_tags.sql
-- Description: Adds immutable column to manifests table to support immutable tag enforcement

ALTER TABLE manifests ADD COLUMN IF NOT EXISTS immutable BOOLEAN DEFAULT FALSE NOT NULL;
