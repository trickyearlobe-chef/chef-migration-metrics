# Getting ownership in from where it is already written down

**As the administrator setting this up, I need to load ownership from the system that already
holds it, because nobody is going to assign thousands of repositories by hand and an
ownership record that is entered twice is a record that will be wrong in one of the two
places.**

The organisation already knows who owns what — in an asset database, a spreadsheet somebody
maintains, an export from a service catalogue. The job is to keep getting it across from
there.

## What I need

To load it from a file, or straight from the database that holds it. For a database, to browse
what a connection can actually see and pick a table, or write a query when the shape is
awkward — because the person configuring this usually cannot inspect that database any other
way, and writing a query blind against a schema you cannot see is guesswork.

### Finding my way around a database I have never seen

The connection comes from somebody else, and so does the password inside it. I am working blind
in somebody else's system.

**The connection has to name its database.** An account that can enumerate every database on a
server is a broader grant than the job needs, and whoever composed the connection already knew
which one they meant. If I need two, that is two connections.

**Then the tables in it, and the views as well.** In a system of record the thing I want is
often a view somebody already built for exactly this, and if I am only shown tables I will go
and rebuild it badly.

**Then the fields, with sample data in them.** Names lie. A column called owner might hold a
team, a person, a login, or nothing at all in nine rows out of ten, and the name will not tell
me which. Seeing what is actually in there is how I judge where the data I need sits.

**Something guessing the field names for me**, which already helps — as long as I can see what
it chose and change it. A guess I cannot override is worse than no guess.

To say which column means what — who the owner is, what they own, how to contact them — since
no two of these systems agree on naming.

To tidy values on the way in without editing the source: strip a mail domain, pull a team name
out of a longer string. The source belongs to somebody else and I cannot make them change it.

**To see what it will do before it does it.** A preview of what would be created and changed,
against real rows. I will not point an import at thousands of assignments on trust.

To have the rows it could not use handed back to me as a worklist — which row, and what was
wrong with it — so I can get the source fixed rather than silently importing three quarters of
it.

**To try the row filter and watch it work before I commit anything.** These exports are usually
one consolidated list with a column saying whether a row is a node, a repository, or something I
do not care about. I want to apply that filter and see what comes back — the right things, and
roughly the right number of them. Committing first and inspecting afterwards is the wrong way
round.

To be able to run it again on a schedule once I trust it, and to see whether the source is
getting better or worse. **That decision turns on having watched it run once, including how long
it took** — a job that takes forty minutes is a different proposition from one that takes four.

**To load the whole thing in one go.** The source is one list, and it can run to a hundred and
fifty thousand records or more. Splitting it — by hand, or by writing filters that between them are
supposed to cover everything exactly once — is a job with no way to check it was done right, and
it has to be redone every time the source is refreshed. A cap I have to work around is a cap
that will eventually be worked around wrongly.

Not to type a password into an import screen. The password is a stored credential; the rest of
the connection I need to be able to see — [connecting to a database that is not
mine](ownership-connection.md).

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

**Typing my own query is what I need today, and it is not what should be there in the end.** It
is query text entered through a screen and run against a database whose credentials belong to
somebody else. But whatever replaces it has to leave me able to reach data no table listing
exposes, or it moves the problem into a ticket queue and I am blind again.

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

**A known gap.** What kind of thing is being imported
is chosen once for the whole run, not read from a column. A source table holding several kinds
of asset has to be imported once per kind, using a row filter to select each — and nothing on
screen says so. Getting it wrong writes assignments against the wrong kind of thing. No test
covers this because the behaviour is not wrong, only silent.

That a connection has to name its database is pinned where the connection is set up, so the
refusal reaches whoever composed it: a connection [naming no
database](internal/secrets/database_url_test.go#TestDatabaseURL_RejectsAConnectionThatNamesNoDatabase)
is refused, as is [one for a driver we cannot
open](internal/secrets/database_url_test.go#TestDatabaseURL_RejectsADriverWeCannotUse), while
[the forms the screen documents are
accepted](internal/secrets/database_url_test.go#TestDatabaseURL_AcceptsTheFormsTheImportScreenDocuments).
The refusal [never quotes the connection
back](internal/secrets/database_url_test.go#TestDatabaseURL_RefusalNeverQuotesTheValue), which
matters because one often arrives with a password already written into it. The same refusal is
repeated [at the point of
use](internal/ownershipsql/dsn_database_test.go#TestEntryPointsRefuseAConnectionWithoutADatabase),
so a connection set up before this existed cannot slip through.

That views are offered alongside tables — usually where the thing worth importing already lives
— is pinned [for every driver we
support](internal/ownershipsql/list_tables_test.go#TestListTablesQuery_IncludesViewsNotJustTables).

**Nothing proves a commit from a database source.** Profiling and preview are exercised against
a real database through the full request path; committing uses the same path and the same writer
as a file import, and has been reasoned about rather than watched. Treat the first real database
commit as unproven.

**Nothing proves a finished run says how long it took**, which is the fact the decision to
schedule turns on. That gap is [held on purpose in this journey's own
suite](internal/webapi/ownership_intake_journey_test.go#TestJourney_ARunSaysHowLongItTook)
rather than described here and forgotten: it fails until somebody closes it.

**Nothing proves I can try the row filter before committing, from a database source.** The
checking step exists, but whether it is reachable when the source is a database rather than a
file is asserted by nothing.

**Nothing proves the field-name guessing helps.** A guess that is usually wrong is worse than
none, because it turns mapping into auditing, and nothing here tells the two apart.

**Nothing proves that looking around cannot write.** Listing, sampling and filtering are
described as ways to find your way, and finding your way is only safe if that is true.

**Nothing proves the source is right.** Everything here is about loading faithfully and
reporting what could not be loaded. An asset database that is confidently out of date imports
cleanly and attributes work to people who left.

**The load-bearing assumption:** that the identity a source names can be matched to a person the
tool already knows about. That is the whole reason the next journey exists — see [one person,
many names](ownership-identity.md).
