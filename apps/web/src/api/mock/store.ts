/* =============================================================================
   Mock auth store — in-memory with sessionStorage persistence.
   This file is an implementation detail of the mock client. Nothing outside
   src/api/ should import from here; use the authApi export from src/api/auth.ts.
   ============================================================================= */

import type { Role } from "../generated/audit";
import type { CurrentUser } from "../auth.types";

// --------------------------------------------------------------------------
// User record (extended with password for mock validation)
// --------------------------------------------------------------------------

export interface MockUser {
  id: string;
  email: string;
  /** Plain-text for mock purposes only. Real client never sees passwords. */
  password: string;
  role: Role;
  tier: string;
}

// --------------------------------------------------------------------------
// Session record (stands in for httpOnly cookies)
// --------------------------------------------------------------------------

export interface MockSession {
  userId: string;
  csrfToken: string;
}

// --------------------------------------------------------------------------
// Storage keys
// --------------------------------------------------------------------------

const KEY_USERS = "gs.mock.users";
const KEY_SESSION = "gs.mock.session";

// --------------------------------------------------------------------------
// Seed data (contract §3)
// --------------------------------------------------------------------------

const SEED_USERS: MockUser[] = [
  {
    id: "seed-returning",
    email: "returning@example.com",
    password: "Str0ng-Passw0rd!",
    role: "user",
    tier: "free",
  },
  {
    id: "seed-taken",
    email: "taken@example.com",
    password: "Str0ng-Passw0rd!",
    role: "user",
    tier: "free",
  },
];

// --------------------------------------------------------------------------
// User store helpers
// --------------------------------------------------------------------------

function loadUsers(): MockUser[] {
  try {
    const raw = sessionStorage.getItem(KEY_USERS);
    if (raw) return JSON.parse(raw) as MockUser[];
  } catch {
    // corrupted storage — fall through to seed
  }
  return [...SEED_USERS];
}

function saveUsers(users: MockUser[]): void {
  sessionStorage.setItem(KEY_USERS, JSON.stringify(users));
}

export function getUsers(): MockUser[] {
  return loadUsers();
}

export function findUserByEmail(email: string): MockUser | undefined {
  return getUsers().find((u) => u.email.toLowerCase() === email.toLowerCase());
}

/** Creates a user only if they don't already exist. Returns the user either way. */
export function upsertUser(email: string, password: string, role: Role = "user"): MockUser {
  const existing = findUserByEmail(email);
  if (existing) return existing;
  const users = getUsers();
  const user: MockUser = {
    id: `mock-${crypto.randomUUID()}`,
    email,
    password,
    role,
    tier: "free",
  };
  users.push(user);
  saveUsers(users);
  return user;
}

// --------------------------------------------------------------------------
// Session helpers
// --------------------------------------------------------------------------

function generateCsrfToken(): string {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function getSession(): MockSession | null {
  try {
    const raw = sessionStorage.getItem(KEY_SESSION);
    if (raw) return JSON.parse(raw) as MockSession;
  } catch {
    // corrupted
  }
  return null;
}

export function createSession(userId: string): MockSession {
  const session: MockSession = { userId, csrfToken: generateCsrfToken() };
  sessionStorage.setItem(KEY_SESSION, JSON.stringify(session));
  return session;
}

export function clearSession(): void {
  sessionStorage.removeItem(KEY_SESSION);
}

export function sessionToCurrentUser(session: MockSession): CurrentUser | null {
  const users = getUsers();
  const user = users.find((u) => u.id === session.userId);
  if (!user) return null;
  return { id: user.id, email: user.email, role: user.role, tier: user.tier };
}
