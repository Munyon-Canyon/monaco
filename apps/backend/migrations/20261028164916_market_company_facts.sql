CREATE TABLE company_facts (
  company_key text PRIMARY KEY,
  shares_outstanding numeric(20, 0) NOT NULL CHECK (shares_outstanding > 0),
  as_of date NOT NULL,
  source text NOT NULL CHECK (source <> ''),
  updated_at timestamptz NOT NULL
);

-- Shares outstanding from each company's latest 10-Q or 10-K cover page, read from the SEC's
-- dei:EntityCommonStockSharesOutstanding on 2026-10-28; source is the filing. company_key is the lowercased
-- xStocks display name. Alphabet, Meta and Coinbase report several share classes on the cover, so they stay
-- unseeded and show a dash for market cap until their classes are summed by hand.
INSERT INTO company_facts (company_key, shares_outstanding, as_of, source, updated_at)
VALUES
  -- 10-Q cover page, filed 2026-07-31
  ('apple', 14594180000, DATE '2026-07-17', 'https://www.sec.gov/Archives/edgar/data/320193/000032019326000020/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-K cover page, filed 2026-07-29
  ('microsoft', 7425545491, DATE '2026-07-23', 'https://www.sec.gov/Archives/edgar/data/789019/000119312526323660/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-07-31
  ('amazon.com', 10786313572, DATE '2026-07-22', 'https://www.sec.gov/Archives/edgar/data/1018724/000101872426000026/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-26
  ('nvidia', 24100000000, DATE '2026-08-21', 'https://www.sec.gov/Archives/edgar/data/1045810/000104581026000075/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-10-22
  ('tesla', 3952757946, DATE '2026-10-15', 'https://www.sec.gov/Archives/edgar/data/1318605/000162828026066775/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-10-21
  ('netflix', 4106325903, DATE '2026-09-30', 'https://www.sec.gov/Archives/edgar/data/1065280/000106528026000291/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-06
  ('jpmorgan chase', 2658186195, DATE '2026-06-30', 'https://www.sec.gov/Archives/edgar/data/19617/000162828026054343/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-05
  ('the walt disney', 1726686902, DATE '2026-07-29', 'https://www.sec.gov/Archives/edgar/data/1744489/000174448926000057/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-28
  ('walmart', 7933746241, DATE '2026-08-26', 'https://www.sec.gov/Archives/edgar/data/104169/000010416926000154/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-05
  ('amd', 1632475042, DATE '2026-07-29', 'https://www.sec.gov/Archives/edgar/data/2488/000000248826000123/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-07-24
  ('intel', 5044000000, DATE '2026-07-17', 'https://www.sec.gov/Archives/edgar/data/50863/000005086326000157/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-10-27
  ('paypal', 830658298, DATE '2026-10-21', 'https://www.sec.gov/Archives/edgar/data/1633917/000163391726000088/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-08-27
  ('salesforce', 823000000, DATE '2026-08-20', 'https://www.sec.gov/Archives/edgar/data/1108524/000110852426000190/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00'),
  -- 10-Q cover page, filed 2026-09-11
  ('oracle', 3023736000, DATE '2026-09-07', 'https://www.sec.gov/Archives/edgar/data/1341439/000119312526389274/',
    TIMESTAMPTZ '2026-10-28 12:00:00+00');
