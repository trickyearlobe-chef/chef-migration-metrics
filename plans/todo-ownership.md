# Ownership — ToDo

Ownership only has to be right where somebody has to act. Measure the list before
building anything that makes matching cleverer — it may be short enough to do by hand.

- [ ] **Make a large import survivable.** A commit is one synchronous request with nothing
  transactional about it, so at full size it holds a connection open past most proxy
  timeouts, and a timeout part way through leaves rows imported with no record of where it
  stopped. Batch it, or make it resumable.

- [ ] **Download the match report as CSV.** It is on screen only.

- [ ] **Gate the committer flow on internal identities.** On a vendored cookbook it invents
  owners from external contributors and seeds their commit addresses as aliases, where they
  go on to match import rows. Needs the set of internal email domains, which is configured
  nowhere.
