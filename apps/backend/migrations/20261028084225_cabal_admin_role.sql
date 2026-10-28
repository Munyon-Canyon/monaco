ALTER TABLE cabal_members DROP CONSTRAINT cabal_members_role_check;

ALTER TABLE cabal_members ADD CONSTRAINT cabal_members_role_check CHECK (role IN ('creator', 'admin', 'member'));

ALTER TABLE cabal_members ADD CONSTRAINT cabal_members_admin_votes CHECK (role <> 'admin' OR can_vote);
