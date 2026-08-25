# Connecting to a database that is not mine

**As the administrator setting up an import, I need to see what I am connecting with, because
when it does not work the only thing an encrypted connection string lets me do is guess.**

The connection belongs to somebody else: their server, their account, their rules. I am handed a
set of details on a ticket and told to make it work. Anything I cannot see is something I cannot
check.

## What I need

**Only the password out of sight.** The address, the database, the account and the domain in
plain view, and editable. When a connection fails I have to be able to read what was actually
sent. Hiding all of it to protect one part of it makes every failure unreadable.

**The password, and only the password, put in for me — correctly.** It is the one value I never
see, so it is the one I can never check. Everything else, including the account and the domain
in front of it, sits in the string where I can read it and fix it myself. Punctuation in a
password fails invisibly.

**I say where it goes, and the screen tells me how to say it.** I mark the spot myself, because
a connection can want its password somewhere nobody could have guessed. But a marker I am
expected to know about and can read nowhere is just a new thing to get wrong — the screen has to
show me how to write it. If I leave it out, refuse me and say so. Do not decide for me and send
something I did not write.

**To be shown what will actually be sent, with the password masked.** If I can read the composed
connection I can see in one glance whether the account,
the domain or the punctuation came out wrong — which is the entire question I am otherwise left
guessing at.

**A connection proposed, not imposed.** Show me one that would work for the kind of database I
picked, filled in with what I have already told you — then let me change any of it. I often have
a string from somebody else's tooling that I would rather paste in whole.

**To test it before going any further.** Asking for the list of tables is not a connection test.
When that comes back empty or angry I cannot tell whether the account is wrong, the network is
closed, the database name is wrong, the string is malformed, or the account is not one the
database checks for itself — and those are five different people to go and talk to.

**To be told when the account is not the database's to check.** An account with something in
front of it — a domain, a machine name, a workgroup, or just a dot — is not checked by the
database at all. It is handed on to whatever keeps that list of people, and if that thing is not
trusted where the database lives, or cannot be reached from there, I am refused for a reason
that has nothing to do with my password and belongs to a different team again. This is the shape
of account I have actually been given, so it is not a corner case. It is also the one refusal
that does not repeat my account back to me, which makes seeing the composed connection the only
thing I have left.

**A failure that tells me which of the five it was**, in the words of whatever refused me. A
message that has been tidied into "could not connect" has thrown away the only thing in it worth
having. Two different things can refuse me — the thing that reads the string, and the server —
and I want to hear from both, as they put it — **except for my password, which must never come
back to me in a message.** Whatever refused me may well quote the whole connection in its
complaint, and everything I am shown ends up somewhere I did not choose: a screenshot, a support
bundle, a log. The one value that was hidden everywhere else
must not reappear in the one place I am reading carefully.

## The decisions behind it

**The password is the only secret.** A host name, a database name and an account name are
configuration: they appear in tickets, in runbooks and in the logs of everything else that talks
to that server. Treating them as secret buys nothing and costs time every time somebody has to
work out what was sent.

**Escaping is owed only where seeing is impossible.** The tool escapes the password because
nobody can inspect it. It must not quietly rewrite anything the administrator can see: typing an
account one way and sending it another is the same unreadable failure in a new place.

**How to escape depends on the form of the string, and getting that backwards fails silently.**
The two shapes these connections come in want different treatment for the same punctuation, so
the tool has to recognise which it was handed. Applying one form's rule to the other produces a
string that looks right and is refused.

**Testing is a separate act from using.** A test that happens as a side effect of asking for
something else cannot say what failed, because it was not trying to find out.

**What the drivers say is worth passing on, and is not enough on its own.** Measured, not
assumed: for SQL Server given a web-style connection, the string is checked before anything is
dialled and the complaint comes back immediately — but hand the same driver a settings-style
connection, or use PostgreSQL at all, and nothing is checked until it tries to connect. So their
words are the best available and must be passed through with nothing tidied away — save the
password, which is taken out of them wherever it appears, however it appears, because a driver
quoting the string it was handed does not know which part of it was a secret. The checks this
tool needs of its own — that a database is named — cannot be delegated to them. Even the useful
complaint is thin: it says the string could not be read, never which character defeated it, which
is why seeing the composed string matters more than the message does.

## What proves it

Nothing here is built yet. What has to be true is enumerated in
[the suite for this journey](internal/webapi/ownership_connection_journey_test.go#TestJourney_OnlyThePasswordIsOutOfSight),
which is red until it is.

**Nothing proves the escaping is right until it is measured against a real server, and it must
not be reasoned about.** Working out what a driver accepts by argument produces confident wrong
answers. There is a SQL Server to try it against, and the cases that matter are known: an account with a domain in front of it,
and a password with punctuation in it. Anything short of running those two against a server that
really refuses or really admits them is a guess wearing a test's clothes.

**Nothing here can prove an account with something in front of it works.** The database we try
these against belongs to nobody's list of people, and it will not let us invent an account that
looks as though it does. So that is exercised for the first time in the estate where it is
deployed, on the account actually used there, and the refusal seen here is not the refusal
seen there.

**Nothing proves a proposed connection is a good starting point.** A suggestion that is usually
wrong is worse than none, because it turns setting up a connection into correcting one.

**The load-bearing assumption:** that the account can reach the server from where this tool runs
at all. One name can answer on several addresses, where some refuse and some time out. Until the
route is open, every failure above looks the same as a firewall, and no amount of getting the
string right will change it.
