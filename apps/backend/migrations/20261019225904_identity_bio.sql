ALTER TABLE users ADD COLUMN bio text CHECK (char_length(bio) <= 160);
