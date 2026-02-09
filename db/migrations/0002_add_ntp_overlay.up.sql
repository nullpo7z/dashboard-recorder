ALTER TABLE tasks ADD COLUMN time_overlay BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN time_overlay_config TEXT NOT NULL DEFAULT 'bottom-right' CHECK(time_overlay_config IN ('top-left', 'top-right', 'bottom-left', 'bottom-right'));
