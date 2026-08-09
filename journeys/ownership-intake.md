# Getting ownership in from where it is already written down

**As the administrator setting this up, I need to load ownership from the system that already
holds it, because nobody is going to assign four thousand repositories by hand and an
ownership record that is entered twice is a record that will be wrong in one of the two
places.**

This organisation already knows who owns what. It is in an asset database, or a spreadsheet
somebody maintains, or an export from a service catalogue. What it is not in is this tool. The
job is to get it across and keep getting it across, not to become a second place where the
truth is typed.

## What I need

To load it from a file, or straight from the database that holds it. For a database, to browse
what a connection can actually see and pick a table, or write a query when the shape is
awkward — because the person configuring this usually cannot inspect that database any other
way, and writing a query blind against a schema you cannot see is guesswork.

To say which column means what — who the owner is, what they own, how to contact them — since
no two of these systems agree on naming.

To tidy values on the way in without editing the source: strip a mail domain, pull a team name
out of a longer string. The source belongs to somebody else and I cannot make them change it.

**To see what it will do before it does it.** A preview of what would be created and changed,
against real rows. I will not point an import at four thousand assignments on trust.

To have the rows it could not use handed back to me as a worklist — which row, and what was
wrong with it — so I can get the source fixed rather than silently importing three quarters of
it.

To be able to run it again on a schedule once I trust it, and to see whether the source is
getting better or worse.

Not to type a password into an import screen. The connection is a stored credential.

## The decisions behind it

**Importing is an administrator's act, not an operator's.** It rewrites who is accountable for
everything, at scale, in one action. An operator can read all of it and change none of it.

**A rejection list is a statement about the source as it stands now, so each run replaces the
last.** If rejections accumulated, the list could never be worked down to empty — and empty is
the only state that makes it worth opening. A row fixed at source must stop being reported.

**Each source's rejections are its own.** Two imports are two systems with two sets of people
to chase; one clearing the other's findings would lose work.

**The preview and the commit go through the same path.** The point of a preview is that it
tells you what the commit will do, and it can only do that if it is the same code answering.

## What proves it

That import and duplicate resolution are refused to an operator is pinned directly — [import
refuses](internal/webapi/handle_ownership_admin_only_test.go#TestOwnershipImport_RefusesAnOperator),
[duplicate resolution
refuses](internal/webapi/handle_ownership_admin_only_test.go#TestOwnershipDuplicates_RefuseAnOperator),
and [an administrator is
admitted](internal/webapi/handle_ownership_admin_only_test.go#TestOwnershipAdminEndpoints_AdmitAnAdmin) so
the check cannot be satisfied by refusing everybody.

The rejection rules are pinned by the cases that motivated them: a row fixed at source [stops
being
reported](internal/datastore/ownership_import_rejections_functional_test.go#TestFunctional_ImportRejections_ReplaceTheSetRatherThanAppend)
on the next run, and one import's findings [do not clear
another's](internal/datastore/ownership_import_rejections_functional_test.go#TestFunctional_ImportRejections_AreScopedToTheirImport).
Both need a real database and run under their own build tag.

Column mapping is pinned across the awkward cases — [every field
mapped](internal/ownershipimport/mapper_test.go#TestMapper_MapsEveryField), the original values
[carried alongside the mapped
ones](internal/ownershipimport/mapper_test.go#TestMapper_CarriesRawValuesAlongsideMapped) so a
rejection can quote what was actually in the row, a display name [defaulting to what the source
said before any tidying](internal/ownershipimport/mapper_test.go#TestMapper_DisplayNameDefaultsToThePreSlugifyOwner),
and joining columns together [not silently collapsing an empty
one](internal/ownershipimport/mapper_test.go#TestMapper_ConcatDoesNotCollapseEmptySegments), which
would produce a plausible wrong name rather than a visible gap. The tidying rules are pinned
including their refusals — a rule that cannot be compiled [is rejected up
front](internal/ownershipimport/transform_test.go#TestCompileTransforms_Rejects) rather than
failing per row, a pattern that matches nothing [yields empty rather than
guessing](internal/ownershipimport/transform_test.go#TestRegexExtract_EmptyOnNoMatchOrNoCaptureGroup),
and stripping a mail domain [leaves an address literal
alone](internal/ownershipimport/transform_test.go#TestStripDomain_LeavesIPLiteralsUnchanged).

**A known gap, and it has already caused a wrong import.** What kind of thing is being imported
is chosen once for the whole run, not read from a column. A source table holding several kinds
of asset has to be imported once per kind, using a row filter to select each — and nothing on
screen says so. Getting it wrong writes assignments against the wrong kind of thing, which
happened on 2026-08-03. No test covers this because the behaviour is not wrong, only silent.

**Nothing proves a commit from a database source.** Profiling and preview are exercised against
a real database through the full request path; committing uses the same path and the same writer
as a file import, and has been reasoned about rather than watched. Treat the first real database
commit as unproven.

**Nothing proves the source is right.** Everything here is about loading faithfully and
reporting what could not be loaded. An asset database that is confidently out of date imports
cleanly and attributes work to people who left.

**The load-bearing assumption:** that the identity a source names can be matched to a person the
tool already knows about. That is the whole reason the next journey exists — see [one person,
many names](ownership-identity.md).
