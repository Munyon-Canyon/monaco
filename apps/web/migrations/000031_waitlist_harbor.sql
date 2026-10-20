-- The cabal race: how many people signed up as each cabal, for the public counter at /api/harbor.
--
-- The API roles cannot read waitlist, so this function is the only way to see a count, and it returns
-- totals and nothing else. A row with no cabal (from before 000030, or from a caller that skipped the
-- question) has not picked a side, so it counts as undecided and the five add up to total.
-- handleHarbor() in lib/waitlist.js reads exactly these six keys.

CREATE OR REPLACE FUNCTION waitlist_harbor() RETURNS JSON
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
  SELECT json_build_object(
    'cavemen', count(*) FILTER (WHERE cabal = 'cavemen'),
    'pilates', count(*) FILTER (WHERE cabal = 'pilates'),
    'cadena', count(*) FILTER (WHERE cabal = 'cadena'),
    'degens', count(*) FILTER (WHERE cabal = 'degens'),
    'undecided', count(*) FILTER (WHERE cabal = 'undecided' OR cabal IS NULL),
    'total', count(*)
  )
  FROM waitlist
$$;

REVOKE ALL ON FUNCTION waitlist_harbor() FROM PUBLIC;

-- Supabase API roles exist only on hosted Supabase, not local Compose Postgres.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
    REVOKE ALL ON FUNCTION waitlist_harbor() FROM anon, authenticated;
    GRANT EXECUTE ON FUNCTION waitlist_harbor() TO anon;
  END IF;
END
$$;
