UPDATE cabals SET join_mode = 'request';
ALTER TABLE cabals DROP CONSTRAINT cabals_join_mode_check;
ALTER TABLE cabals ADD CONSTRAINT cabals_join_mode_check CHECK (join_mode IN ('request'));
