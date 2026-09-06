# Acceptance criteria: Public landing page for unauthenticated visitors

## Acceptance criteria (summary)

- [ ] An unauthenticated visitor loading `/` sees the landing page with the regimen hero,
      the marketing sections, and the header menu
- [ ] The header shows marketing anchors plus Log in / Get started, and no app destinations
- [ ] Below the mobile breakpoint the menu collapses to a hamburger that opens and closes
- [ ] Any auth action opens the auth modal over the page without navigating away
- [ ] A visitor can register with email and password and lands authenticated on the dashboard
- [ ] A returning visitor can log in with email and password and lands on the dashboard
- [ ] "Continue with Google" completes OAuth2 and lands the visitor on the dashboard
- [ ] Invalid credentials and duplicate registrations return a generic message that does not
      reveal whether an email is registered
- [ ] Field-level validation errors appear before any network call is made
- [ ] Auth-service failures and rate limiting produce a readable error, never a stuck spinner
- [ ] The modal traps focus, closes on Escape / backdrop / close button, and returns focus
      to the control that opened it
- [ ] An already-authenticated visitor hitting `/` is redirected to the dashboard
- [ ] The page renders correctly in light and dark themes, and honours `prefers-reduced-motion`
- [ ] The silhouette asset is original and depicts no real person or existing character
- [ ] The page renders correctly in `en-US` and `vi-VN` with no clipped or overflowing text
- [ ] No user-facing string is hardcoded; every key exists in both locale catalogs
- [ ] Locale is detected from `Accept-Language`, and `?lng=vi` forces Vietnamese for testing
- [ ] `<html lang>` reflects the active locale
- [ ] Every design value used resolves to a token; no raw colors, sizes or durations appear

## Acceptance criteria (scenarios)

### Scenario: unauthenticated visitor sees the landing page and its menu

**Given** I am not authenticated
**When** I load `/`
**Then** the hero displays the regimen lines "100 PUSH-UPS", "100 SIT-UPS", "100 SQUATS"
and "10KM RUN" with the "EVERY SINGLE DAY" punchline
**And** the original silhouette figure renders behind the type
**And** the header menu shows Features, How it works and Pricing, plus Log in and Get started
**And** no authenticated app destination is shown or linked anywhere on the page

### Scenario: menu collapses to a hamburger on a narrow viewport

**Given** I am viewing the landing page below the mobile breakpoint
**Then** the marketing anchors are hidden behind a hamburger control
**When** I activate the hamburger
**Then** the menu expands showing every anchor plus Log in and Get started
**And** activating it again, or selecting any item, collapses the menu

### Scenario: menu anchor scrolls to the matching section

**Given** I am on the landing page
**When** I select the "How it works" menu item
**Then** the page scrolls to the "How it works" section
**And** the scroll is instant rather than animated when `prefers-reduced-motion` is set

### Scenario: new visitor registers and reaches the dashboard

**Given** I am not authenticated and no account exists for `new@example.com`
**And** I have opened the auth modal on the Sign up tab
**When** I submit `new@example.com` with a password meeting the policy
**Then** auth-service creates the account and sets the session cookies
**And** I am redirected to the dashboard as an authenticated user
**And** the modal is dismissed

### Scenario: returning visitor logs in and reaches the dashboard

**Given** an account exists for `returning@example.com`
**And** I have opened the auth modal on the Log in tab
**When** I submit that email with the correct password
**Then** the session cookies are set and I am redirected to the dashboard

### Scenario: visitor authenticates with Google

**Given** I am not authenticated
**And** I have opened the auth modal
**When** I select "Continue with Google" and approve access at Google
**Then** I am returned to the application with session cookies set
**And** I am redirected to the dashboard

### Scenario: auth modal opens over the page and can be dismissed without losing position

**Given** I have scrolled to the Features section
**When** I activate Log in and then dismiss the modal with Escape
**Then** the modal closes without navigating away
**And** the page is still scrolled to Features
**And** keyboard focus returns to the Log in control that opened the modal

### Scenario: focus stays inside the modal while it is open

**Given** the auth modal is open
**When** I move forward through the focusable elements with the keyboard past the last one
**Then** focus wraps to the first focusable element inside the modal
**And** focus never reaches the page behind the modal

### Scenario: switching modal tabs clears errors from the other tab

**Given** the auth modal is on the Log in tab showing a failed-login error
**When** I switch to the Sign up tab
**Then** the login error is cleared
**And** the Sign up fields are empty and unvalidated

### Scenario: already-authenticated visitor is redirected away from the landing page

**Given** I hold a valid session
**When** I navigate to `/`
**Then** I am redirected to the dashboard without the landing page being shown as a
final destination

### Scenario: invalid credentials do not reveal whether the email is registered

**Given** the auth modal is on the Log in tab
**When** I submit an email and password combination that does not authenticate
**Then** a single generic message such as "Email or password is incorrect" is shown
**And** the message is identical whether or not that email has an account
**And** the password field is cleared while the email field keeps its value

### Scenario: registering an already-registered email does not confirm the account exists

**Given** an account already exists for `taken@example.com`
**When** I submit that email on the Sign up tab
**Then** the response does not state that the email is already registered
**And** I am shown the same neutral next step a successful registration would produce,
per the account-enumeration protection in SRS-AUTH-001 §5

### Scenario: client-side validation blocks a malformed submission before any request

**Given** the auth modal is open on the Sign up tab
**When** I submit an email without an `@` or a password below the policy minimum
**Then** field-level validation messages identify each offending field
**And** no request is sent to auth-service
**And** the submit control stays disabled until the fields are valid

### Scenario: auth-service being unreachable surfaces an error instead of hanging

**Given** the auth modal is open and auth-service is unreachable
**When** I submit valid credentials
**Then** the loading state ends
**And** a message invites me to try again
**And** the modal stays open with the email field's value intact

### Scenario: rate-limited login attempts are reported clearly

**Given** I have exceeded the login rate limit for my address (SRS-AUTH-001 NFR-05)
**When** I submit credentials again
**Then** auth-service responds 429
**And** I am shown a message explaining that too many attempts were made and to wait
**And** the submit control is disabled for the cool-down period

### Scenario: cancelling Google authorization returns the visitor without a session

**Given** I have selected "Continue with Google"
**When** I deny access at Google's consent screen
**Then** I am returned to the landing page with no session cookies set
**And** a non-blocking message notes that sign-in was not completed
**And** I remain able to use email and password instead

### Scenario: a Vietnamese-language browser gets the Vietnamese landing page

**Given** I am not authenticated
**And** my browser sends an `Accept-Language` header preferring Vietnamese
**When** I load `/`
**Then** all landing page copy, menu items and modal labels render in Vietnamese
**And** `<html lang>` is `vi-VN`
**And** no English string remains visible anywhere on the page or in the auth modal

### Scenario: any other language falls back to English

**Given** my browser prefers a language we do not support, such as French
**When** I load `/`
**Then** the page renders in `en-US`
**And** `<html lang>` is `en-US`

### Scenario: the locale override forces Vietnamese for testing

**Given** my browser prefers English
**When** I load `/?lng=vi`
**Then** the page renders in Vietnamese
**And** this works in a production build, because with no language switcher it is
the only way to reproduce a Vietnamese-only defect

### Scenario: Vietnamese text does not break any layout

**Given** the page is rendering in `vi-VN`
**When** I view it at the mobile, tablet and desktop breakpoints
**Then** no heading, button label, nav item or error message is clipped, truncated or
overlapping
**And** stacked diacritics such as `ế ộ ữ` are fully visible rather than cut off by
their container
**And** every face used renders Vietnamese glyphs rather than fallback boxes

### Scenario: the page conforms to the token system

**Given** the landing page implementation
**When** its stylesheets and components are inspected
**Then** every color, size, spacing and duration value resolves to a token defined in
`tokens.css`
**And** no raw hex value, pixel size or millisecond duration appears in a component
**And** the page renders correctly in both light and dark theme
