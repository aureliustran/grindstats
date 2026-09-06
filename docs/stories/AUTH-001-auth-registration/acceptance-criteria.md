# Acceptance criteria: Account registration

## Acceptance criteria (summary)

- [ ] A new email + password registration creates an account with role `User`
- [ ] Email uniqueness is enforced case-insensitively
- [ ] A password under 10 characters is rejected before an account is created
- [ ] A password found in the breached-password check is rejected or flagged (see story
      Open questions for which)
- [ ] No request field can set a role other than `User` on registration
- [ ] First-time Google OAuth login auto-creates a `User` account from the verified claim
- [ ] An OAuth login matching an existing local account's email requires proof of control
      before linking — it never silently merges
- [ ] A verification email is sent on local registration with a single-use, 24h token
- [ ] An unverified account can log in but is restricted to read-only access to its own data
- [ ] Password reset follows request → single-use 1h token → new password, and completing
      it invalidates every existing session
- [ ] Registration, login, and reset responses are indistinguishable regardless of whether
      the target email exists

## Acceptance criteria (scenarios)

### Scenario: registering with a new email creates a User account

**Given** no account exists for `new@example.com`
**When** I submit that email with a password meeting the policy
**Then** an account is created with role `User`
**And** a verification email is sent to that address
**And** I am not treated as SystemAdmin under any circumstance

### Scenario: email uniqueness is case-insensitive

**Given** an account exists for `Taken@Example.com`
**When** I submit `taken@example.com` (different case) on registration
**Then** the system treats it as the same address
**And** the response is identical to the duplicate-registration response in the enumeration
scenario below — it does not reveal that the address is taken

### Scenario: password below the minimum length is rejected

**Given** I am registering
**When** I submit a password shorter than 10 characters
**Then** registration is rejected with a validation error naming the length requirement
**And** no account is created

### Scenario: a breached password is rejected or flagged

**Given** I am registering with a password known to appear in a breach corpus
**When** I submit it
**Then** the system either rejects the submission or surfaces a clear warning before
account creation completes (exact behavior pending the story's open question)
**And** in either case no account silently uses a known-compromised password without the
registrant seeing a warning

### Scenario: an injected role field is ignored on registration

**Given** I am registering
**When** my request payload includes `"role": "SystemAdmin"` (or any non-default role)
**Then** the field is ignored or the request is rejected
**And** if an account is created, its role is `User`

### Scenario: first Google login auto-creates an account

**Given** no local or OAuth account exists for my Google identity
**When** I complete Google's consent screen with `state` and PKCE validated
**Then** a `User` account is created using the verified email from the OAuth claim
**And** I do not need to separately register with a password

### Scenario: OAuth login matching an existing local account requires proof of control

**Given** a local account already exists for `shared@example.com`
**When** I complete Google OAuth using that same email
**Then** the accounts are not silently merged
**And** I am required to log in to the local account, or confirm via its verified email,
before the identities are linked

### Scenario: unverified account is read-only on its own data

**Given** I registered locally and have not clicked the verification link
**When** I log in and attempt to write data (e.g. log a meal or a training set)
**Then** the write is rejected
**And** reading my own existing data still succeeds
**And** I am told that verifying my email unlocks writing

### Scenario: verification token expires after 24 hours

**Given** I registered more than 24 hours ago and have not verified
**When** I follow the verification link
**Then** the token is rejected as expired
**And** I am offered a way to obtain a new one (see story Open questions)

### Scenario: verification token is single-use

**Given** I have already successfully verified my email once
**When** the same verification link is visited again
**Then** it is rejected as already used
**And** my account remains verified (the second attempt does not un-verify it)

### Scenario: password reset completes and logs out every session

**Given** I have requested a password reset and hold a valid, unused, unexpired reset token
**When** I submit a new password with that token
**Then** my password is updated
**And** every existing session for my account is invalidated (logout-all is triggered)
**And** the reset token cannot be used again

### Scenario: expired or already-used reset token is rejected

**Given** a reset token that is either older than 1 hour or already consumed
**When** I attempt to use it to set a new password
**Then** the request is rejected
**And** my existing password remains unchanged
**And** no session is invalidated as a side effect of the rejected attempt

### Scenario: registration response does not confirm an email is already registered

**Given** an account already exists for `taken@example.com`
**When** I submit that email on registration
**Then** the response is the same shape and wording as a successful new registration
**And** nothing in the response, status code, or timing confirms the account already exists

### Scenario: password-reset request does not confirm whether the email exists

**Given** `nobody@example.com` has no account
**When** I request a password reset for that address
**Then** the response is identical to requesting a reset for a real, registered address
**And** no reset email is sent, but the requester cannot tell that from the response
