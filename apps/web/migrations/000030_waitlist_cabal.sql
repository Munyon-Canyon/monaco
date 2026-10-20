-- Which cabal a visitor signed up as: cavemen, pilates, cadena, degens or undecided.
--
-- Null means no answer: every row from before this migration, and any caller that skips the
-- landing page's question. Keep the list in sync with CABALS in lib/waitlist.js and the options
-- in index.html; test/cabals.test.js checks all three.
--
-- Adding an argument makes a different function rather than replacing join_waitlist(), as in 000027.
-- The six-argument version does the work and the five-argument one stays, delegating to it with no
-- cabal, so a page and a database that are briefly out of step keep working either way.

ALTER TABLE waitlist ADD COLUMN IF NOT EXISTS cabal TEXT;
ALTER TABLE waitlist DROP CONSTRAINT IF EXISTS waitlist_cabal_check;
ALTER TABLE waitlist ADD CONSTRAINT waitlist_cabal_check
  CHECK (cabal IS NULL OR cabal IN ('cavemen', 'pilates', 'cadena', 'degens', 'undecided'));

-- Returns: ok | invalid_email | invalid_twitter | invalid_cabal | invalid_request | rate_limited | busy.
CREATE OR REPLACE FUNCTION join_waitlist(
  p_email TEXT,
  p_cabal TEXT,
  p_twitter TEXT,
  p_source TEXT,
  p_ip_hash TEXT,
  p_user_agent TEXT
) RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_email TEXT := lower(btrim(coalesce(p_email, '')));
  v_twitter TEXT := nullif(regexp_replace(btrim(coalesce(p_twitter, '')), '^@', ''), '');
  v_cabal TEXT := nullif(lower(btrim(coalesce(p_cabal, ''))), '');
BEGIN
  IF length(v_email) > 254 OR v_email !~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]{2,}$' THEN
    RETURN 'invalid_email';
  END IF;
  IF v_twitter IS NOT NULL AND v_twitter !~ '^[A-Za-z0-9_]{1,15}$' THEN
    RETURN 'invalid_twitter';
  END IF;
  IF v_cabal IS NOT NULL AND v_cabal NOT IN ('cavemen', 'pilates', 'cadena', 'degens', 'undecided') THEN
    RETURN 'invalid_cabal';
  END IF;
  IF p_ip_hash IS NULL OR p_ip_hash !~ '^[0-9a-f]{64}$' THEN
    RETURN 'invalid_request';
  END IF;

  IF (SELECT count(*) FROM waitlist WHERE created_at > now() - interval '1 minute') >= 60 THEN
    RETURN 'busy';
  END IF;
  IF (SELECT count(*) FROM waitlist WHERE ip_hash = p_ip_hash AND created_at > now() - interval '1 hour') >= 5 THEN
    RETURN 'rate_limited';
  END IF;

  INSERT INTO waitlist (email, twitter, cabal, source, ip_hash, user_agent)
  VALUES (v_email, v_twitter, v_cabal, nullif(left(btrim(coalesce(p_source, '')), 64), ''), p_ip_hash, left(p_user_agent, 256))
  ON CONFLICT (email) DO UPDATE
    -- Someone signing up again, having answered the first time, should not lose the answer,
    -- and a later one is the one they meant. Same for the handle.
    SET twitter = coalesce(excluded.twitter, waitlist.twitter),
        cabal = coalesce(excluded.cabal, waitlist.cabal);
  RETURN 'ok';
END;
$$;

CREATE OR REPLACE FUNCTION join_waitlist(
  p_email TEXT,
  p_twitter TEXT,
  p_source TEXT,
  p_ip_hash TEXT,
  p_user_agent TEXT
) RETURNS TEXT
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$ SELECT join_waitlist(p_email, NULL::TEXT, p_twitter, p_source, p_ip_hash, p_user_agent) $$;

REVOKE ALL ON FUNCTION join_waitlist(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

-- Supabase API roles exist only on hosted Supabase, not local Compose Postgres.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
    -- The new column is on the same table the anon role must not read directly.
    REVOKE ALL ON waitlist FROM anon, authenticated;
    REVOKE ALL ON FUNCTION join_waitlist(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM anon, authenticated;
    GRANT EXECUTE ON FUNCTION join_waitlist(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) TO anon;
    -- The five-argument version keeps its grant, so a page deployed before this migration carries on working.
  END IF;
END
$$;
