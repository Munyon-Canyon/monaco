ALTER TABLE feed_objects DROP CONSTRAINT feed_objects_kind_check;

ALTER TABLE feed_objects ADD CONSTRAINT feed_objects_kind_check
  CHECK (kind IN ('proposal', 'trade', 'price_move', 'cabal_created', 'member_joined', 'reply', 'threshold'));

ALTER TABLE feed_objects DROP CONSTRAINT feed_objects_ref_type_check;

ALTER TABLE feed_objects ADD CONSTRAINT feed_objects_ref_type_check
  CHECK (ref_type IN ('proposals', 'swaps', 'asset_price_moves', 'cabals', 'cabal_members', 'feed_comments'));
