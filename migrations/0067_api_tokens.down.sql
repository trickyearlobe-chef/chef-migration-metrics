-- Reverses 0067. Every credential is destroyed, which is the honest outcome:
-- there is nothing to preserve but hashes, and a tool holding one should stop
-- working rather than half-working.

DROP TABLE IF EXISTS api_tokens;
