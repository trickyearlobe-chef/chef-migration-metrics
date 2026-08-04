# Archived specifications — historical, NOT authoritative

Nothing in this directory describes the system as it is. These are the specifications as
they stood at commit `a0abd66` (2026-08-04), kept because they hold design reasoning and
ideas worth mining, and for nothing else.

**Do not read these to find out how anything works, and never plan or estimate from them.**
The code is the source of truth. Contracts are tests.

They were archived because they had drifted far enough to be actively misleading: 20,422
lines describing 225,378 lines of code, roughly a sixth of them untouched for two months
while the code moved. They asserted tables, columns, endpoints and processes that do not
exist, and stated plans as though they were states.

Read one only when its subject comes up in new work and you want to know what was
previously considered — then verify everything against code before acting on it.

Live specifications are in `specifications/` and contain user journeys only.
