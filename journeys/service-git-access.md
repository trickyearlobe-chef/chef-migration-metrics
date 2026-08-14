# Letting it reach our git servers

**As the person setting this up, I need it to read our repositories over SSH, and I need to do
both halves of that from a screen — I work through a locked-down desktop with no shell on the
box.**

Their git server has to trust a key from this machine, and this machine has to trust their git
server. Both are done once, at the start, and today both need a terminal.

## What I need

- To see the public keys this machine has, with their filenames, so I can copy one into GitLab
  or Stash.
- To generate one when I want one — because there is none, or because I want one for this tool
  specifically. Once made it is in the list under its own name, and I copy it like any other.
- Not to be asked which key to use when it connects.
- The first connection made so their host key is accepted, after I have seen the fingerprint and
  said yes.
- To be told which half is missing when a clone fails: a key they have never been given, or a
  server this machine does not trust.
- All of it on the screen where I add the git addresses, because that is where I am when I find
  out I need it.

## The decisions behind it

**This tool does not own the key.** The estate owns its keys. Generating one is something I ask
for, never something that happens on its own, and it never replaces a key already there.

**Nothing names a key when it connects.** SSH already works that out from what is on the
machine, and the answer depends on how the box was set up rather than on anything typed here.

**Trusting a host key is a decision, so it is shown and confirmed.** Accepting whatever answers
is how somebody gets in the middle, and nothing asks again afterwards.

## How I would know it worked

I copy a key from this screen into GitLab, accept our server's fingerprint, and a repository
clones — without opening a terminal.

## What proves it

Nothing is built. The list is [the suite for this
journey](internal/webapi/service_git_access_journey_test.go#TestJourney_ICanSeeTheKeysThisMachineHas),
red until it is.

**Nothing proves the key was accepted at the other end.** That is done by hand, in GitLab. All
this can show is what was offered.

**Nothing proves the fingerprint is the one they published.** Only a person can compare it with
what their administrators published.

**Nothing proves which key SSH will offer.** It depends on the machine's own configuration, so a
box with several keys can offer the wrong one.
