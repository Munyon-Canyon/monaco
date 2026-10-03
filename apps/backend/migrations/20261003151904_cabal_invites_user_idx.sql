CREATE INDEX cabal_access_requests_pending_user_idx ON cabal_access_requests (user_id) WHERE status = 'pending';
