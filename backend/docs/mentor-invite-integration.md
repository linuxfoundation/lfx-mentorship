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
  - Anyone else is found by their **exact** LF username or **full** email.
    Partial emails never match.
- Empty state: if the query looks like a full email or a username and nothing
  comes back, show *"No LF account found. Ask them to create one at
  sso.linuxfoundation.org, then invite them by email or username."*

## 3. Send the invite

When the admin picks a candidate, send its LFID:

```
POST /programs/{programId}/members
{ "lfid": "alice", "member_type": "mentor" }
```

Response `201` returns the member with `"status": "invited"`. The backend
emails the invite to the account's primary email.

Inviting by email without picking a candidate also works:
`{ "email": "alice@example.org", "member_type": "mentor" }`. Prefer the LFID
from the picked candidate, though, because the admin has then seen who they
are inviting.

Never send both `user_id` and `lfid`.

## 4. Handle errors

| Status | Meaning | UI |
|---|---|---|
| `400` | Bad input (short search, malformed email), or the program isn't published yet | Inline validation message |
| `401` / `403` | Not signed in, or not a Program Admin of this program | Hide the action for non-admins; otherwise show a generic error |
| `409` | Already invited or already a mentor on this program | *"This person is already on the program."* Point to **Resend invite** if they are `invited` |
| `422` | No LF account for that LFID or email | Same message as the empty state in step 2 |
| `503` | Account lookup is temporarily unavailable | *"Couldn't look up accounts right now. Try again."* Keep the dialog open |

## 5. After a successful invite

- Refresh the Mentors tab list. The new row appears as `invited`.
- Resend and revoke reuse the existing actions:
  - `POST /programs/{id}/members/{memberId}/resend-invite`
  - `PATCH` the member status to `declined`
- The invite landing page from #3171 needs no change. The invitee accepts after
  signing in, and their first sign-in lands on the Mentorship user the invite
  created.

## Testing on dev

- Wait for #262 to merge and the dev backend to pick up the new image.
- Dev only delivers email to `linuxfoundation.org` addresses. Invite one of
  those to test the full email → accept flow.
- Quick checks:
  - search your own LF username → one result;
  - search a name fragment of someone not in Mentorship → no results (expected);
  - invite the same person twice → `409`;
  - invite `nobody-xyz@example.invalid` → `422`.
