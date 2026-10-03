ALTER TABLE assets
  ADD COLUMN ui_multiplier_next_num bigint NULL,
  ADD COLUMN ui_multiplier_next_den bigint NULL,
  ADD COLUMN ui_multiplier_next_at timestamptz NULL,
  ADD CONSTRAINT assets_ui_multiplier_next_whole CHECK (
    (ui_multiplier_next_num IS NULL) = (ui_multiplier_next_den IS NULL)
    AND (ui_multiplier_next_num IS NULL) = (ui_multiplier_next_at IS NULL)
  );
