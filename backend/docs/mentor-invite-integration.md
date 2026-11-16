<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Mentor invite dialog: Self Serve integration

This guide covers wiring the Self Serve "Invite mentor" dialog to the
Mentorship API from [#262](https://github.com/linuxfoundation/lfx-mentorship/pull/262).
A Program Admin can invite anyone with an LF account, whether or not that person
has used Mentorship before. The backend handles account lookup and creating
the Mentorship user, so Self Serve never calls user-service or auth-service.

All paths below are relative to the gateway base,
`https://lfx-api.dev.v2.cluster.linuxfound.info/mentorship/v1` on dev. Both
routes require the caller to be a Program Admin of the program (Heimdall
`writer`), so forward the user's token as the Mentors tab actions already do.

## 1. Add the BFF proxy for the typeahead

Proxy one new read route next to the existing member routes:

```
GET /programs/{programId}/mentor-candidates?search={query}
```

Response `200`:

```json
{
  "data": [
    { "lfid": "alice", "name": "Alice Example", "avatar_url": "https://…" }
  ]
}
```

- Results never include an email. Do not try to add one.
- At most 10 results. `name` and `avatar_url` may be missing; fall back to the
  LFID and an initials avatar.
- `search` must be at least 2 characters (trimmed), otherwise the API returns `400`.

## 2. Build the search field

- Debounce input by about 300 ms. Don't call the API below 2 characters.
- Show each candidate's avatar, name and LFID.
- Helper text: *"Search by name, LF username, or full email address."*
  - Name search only finds people who already use Mentorship.
  - Anyone else is found by their **exact** LF username (case-sensitive) or
    **full** email. Partial emails never match.
- Empty state: if the query is a username and nothing comes back, show *"No LF
  account found."* If it is a full email, offer **Invite by email** instead
  (section 3). A lookup outage with no local matches returns `503`, not an empty list.

## 3. Send the invite

When the admin picks a candidate, send its LFID:

```
POST /programs/{programId}/members
{ "lfid": "alice", "member_type": "mentor" }
```

Response `201` returns the member with `"status": "invited"`. The backend
emails the invite to the person's Mentorship email, or to the account's primary
email if they have never used Mentorship.

Inviting by email without picking a candidate also works:
`{ "email": "alice@example.org", "member_type": "mentor" }`. Prefer the LFID
from the picked candidate, though, because the admin has then seen who they
are inviting.

Never send both `user_id` and `lfid`.

### Inviting someone with no LF account

When a full email finds no candidate, the admin can still invite that address:

```
POST /programs/{programId}/members
{ "email": "new.mentor@example.org", "member_type": "mentor" }
```

Response `201` returns an `invited` member with an empty `user_id`. The backend
emails that address the three steps: create an LF account **with this address**,
create a mentor profile, then answer through the usual invite link. Accepting
works only for an LF account that has the address as a verified email; anyone
else gets a `403`. Inviting the same address again resends the email.

In the Mentors tab list such a row has `"pending_account": true` and shows only
its email. It can be revoked (`PATCH` to `declined`) or deleted, but not approved,
and its email cannot be changed: revoke it and invite the new address instead.
If the invitee asks to join the program while creating their mentor profile,
accepting the invitation approves that request.

## 4. Handle errors

| Status | Meaning | UI |
|---|---|---|
| `400` | Bad input (short search, malformed email), or the program isn't published yet | Inline validation message |
| `401` / `403` | Not signed in, or not a Program Admin of this program | Hide the action for non-admins; otherwise show a generic error |
| `409` | The person is already an active mentor, has asked to join (`requested`), or is `pending` | See below |
| `422` | No LF account for that LFID, or another Mentorship user already holds the account's primary email | Show the message from the response body |
| `503` | Account lookup is temporarily unavailable | *"Couldn't look up accounts right now. Try again."* Keep the dialog open |

Inviting someone whose mentor row is `invited`, `declined` (including a revoked
invite) or `withdrawn` is not an error: the backend sets the row back to
`invited`, returns it with `201` and sends the invite email again. That is also
how an admin resends an invite. A `409` covers the rest:

- `approved`: *"This person is already a mentor on the program."*
- `requested`: point the admin to approving the request.
- `pending`: find their row in the Mentors tab list and act on its status.

## 5. After a successful invite

- Refresh the Mentors tab list. The row appears as `invited`.
- To resend an invite, invite the same person again (section 3).
- To revoke one, `PATCH` the member status to `declined`.
- The invite landing page from
  [linuxfoundation/lfx-self-serve#3171](https://github.com/linuxfoundation/lfx-self-serve/issues/3171)
  handles both kinds of link. The invitee accepts after signing in, and their first
  sign-in lands on the Mentorship user the invite created. For an email
  invitation, a `403` means the signed-in account does not use the invited address.

## Testing on dev

- Dev only delivers email to `linuxfoundation.org` addresses. Invite one of
  those to test the full email → accept flow.
- Quick checks:
  - search your own LF username → one result;
  - search a name fragment of someone not in Mentorship → no results (expected);
  - invite the same person twice → `201` both times, the same row, and a second invite email;
  - invite someone who is already an active mentor → `409`;
  - invite an LFID that does not exist → `422`;
  - invite an allowlisted address with no LF account → `201` with an empty
    `user_id`, and the three-step email arrives.
