-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT
-- ============================================
-- Migration: Mentorship Schema — Initial
-- Source: jobspring-prod-* DynamoDB tables
-- ============================================

CREATE SCHEMA IF NOT EXISTS mentorship;

BEGIN;

SET LOCAL search_path TO mentorship, public;

-- SCHEMA public is required, not cosmetic. CREATE EXTENSION with no SCHEMA
-- clause installs into the first schema on search_path — "mentorship" — which
-- on the shared RDS instance makes pgcrypto's functions unresolvable for every
-- other service, whose search_path is its own schema plus public.
CREATE EXTENSION IF NOT EXISTS "pgcrypto" SCHEMA public;

-- ============================================
-- Trigger: set updated_on on every UPDATE
-- ============================================
CREATE OR REPLACE FUNCTION set_updated_on()
  RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_on = NOW();
  RETURN NEW;
END;
$$;

-- ============================================
-- TABLE: users
-- Source: jobspring-prod-users
-- ============================================
CREATE TABLE IF NOT EXISTS users (
  id          UUID         PRIMARY KEY,
  email       TEXT         UNIQUE,
  lfid        TEXT         UNIQUE,
  name        TEXT,
  given_name  TEXT,
  family_name TEXT,
  avatar_url  TEXT,
  created_on  TIMESTAMPTZ  DEFAULT NOW(),
  updated_on  TIMESTAMPTZ  DEFAULT NOW()
);

-- ============================================
-- TABLE: user_profiles
-- Source: jobspring-prod-user-profiles (type = 'mentor' | 'mentee')
-- Rows where recordKind = 'github-profile-reservation' are excluded.
-- ============================================
CREATE TABLE IF NOT EXISTS user_profiles (
  id                   UUID         PRIMARY KEY,
  user_id              UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  profile_type         TEXT         NOT NULL,              -- mentor | mentee
  slug                 TEXT         UNIQUE,
  first_name           TEXT,
  last_name            TEXT,
  email                TEXT,
  phone                TEXT,
  logo_url             TEXT,
  introduction         TEXT,
  terms_and_conditions BOOLEAN      DEFAULT false,
  number_of_projects   INTEGER      DEFAULT 0,
  address              JSONB,                              -- {country, city, address1, zipCode}
  demographics         JSONB,                              -- {gender, race, age}
  socioeconomics       JSONB,                              -- {income, educationLevel}
  skill_set            JSONB,                              -- {skills[], improvementSkills[], comments}
  profile_links        JSONB,                              -- {resumeLink, linkedinProfileLink, githubProfileLink}
  created_on           TIMESTAMPTZ  DEFAULT NOW(),
  updated_on           TIMESTAMPTZ  DEFAULT NOW()
);

-- ============================================
-- TABLE: programs
-- Source: jobspring-prod-projects
-- lf_project_uid is the LF project parent in the authorization chain.
-- ============================================
CREATE TABLE IF NOT EXISTS programs (
  id                   UUID         PRIMARY KEY,
  name                 TEXT         NOT NULL,
  slug                 TEXT         NOT NULL UNIQUE,
  status               VARCHAR(20)  NOT NULL DEFAULT 'pending',  -- pending | submitted | published | rejected | archived | hidden
  is_paid              BOOLEAN      NOT NULL DEFAULT false,        -- stipend paid to mentees
  description          TEXT,
  logo_url             TEXT,
  website_url          TEXT,
  repo_link            TEXT,
  code_of_conduct      TEXT,
  industry             TEXT,                               -- comma-separated skill tags (raw)
  color                VARCHAR(10),
  lfid                 TEXT,                               -- owner lfid
  cii_project_id       TEXT,
  accept_applications  BOOLEAN      DEFAULT false,
  terms_and_conditions BOOLEAN      DEFAULT false,
  program_term_status  VARCHAR(20),                        -- open | closed (denormalised summary)
  discover_sort_rank   INTEGER      DEFAULT 0,
  amount_raised        NUMERIC(20,2) DEFAULT 0,
  mentee_needs         JSONB,                              -- {mentors[], skills[], programTerms{}, acceptedMentees, graduatedMentees}
  task_templates       JSONB,                              -- default task list for new terms
  created_on           TIMESTAMPTZ  DEFAULT NOW(),
  updated_on           TIMESTAMPTZ  DEFAULT NOW(),
  lf_project_uid       TEXT,
  lf_project_slug      TEXT,
  lf_project_name      TEXT,
  lf_project_logo_url  TEXT,
  CONSTRAINT programs_status_check CHECK (status IN ('pending', 'submitted', 'published', 'rejected', 'archived', 'hidden'))
);

-- ============================================
-- TABLE: program_skills
-- Source: jobspring-prod-projects → menteeNeeds.skills[]
-- Normalised from the embedded skills list on each project.
-- ============================================
CREATE TABLE IF NOT EXISTS program_skills (
  id         UUID  PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id UUID  NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  skill      TEXT  NOT NULL,
  created_on TIMESTAMPTZ DEFAULT NOW(),
  updated_on TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE (program_id, skill)
);

-- ============================================
-- TABLE: program_funding_stats
-- Source: jobspring-prod-projects → amountRaised
-- One row per program (1-to-1).
-- ============================================
CREATE TABLE IF NOT EXISTS program_funding_stats (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id    UUID         NOT NULL UNIQUE REFERENCES programs(id) ON DELETE CASCADE,
  amount_raised NUMERIC(20,2) NOT NULL DEFAULT 0,
  amount_spent  NUMERIC(20,2) NOT NULL DEFAULT 0,
  created_on    TIMESTAMPTZ  DEFAULT NOW(),
  updated_on    TIMESTAMPTZ  DEFAULT NOW()
);

-- ============================================
-- TABLE: program_terms
-- Source: jobspring-prod-program-terms
-- ============================================
CREATE TABLE IF NOT EXISTS program_terms (
  id                    UUID         PRIMARY KEY,
  program_id            UUID         NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  name                  TEXT         NOT NULL,
  status                VARCHAR(20)  NOT NULL DEFAULT 'open',     -- open | closed | deleted
  active_users          INTEGER      DEFAULT 0,
  start_date_time       TIMESTAMPTZ,
  end_date_time         TIMESTAMPTZ,
  application_start_date TIMESTAMPTZ,
  application_end_date   TIMESTAMPTZ,
  created_on            TIMESTAMPTZ  DEFAULT NOW(),
  updated_on            TIMESTAMPTZ  DEFAULT NOW(),
  CONSTRAINT program_terms_status_check CHECK (status IN ('open', 'closed', 'deleted'))
);

-- ============================================
-- TABLE: program_members
-- Source: jobspring-prod-project-members
-- All program participants: program_admins and mentors.
-- Mentees are term-scoped and tracked exclusively via applications.
-- ============================================
CREATE TABLE IF NOT EXISTS program_members (
  id          UUID         PRIMARY KEY,
  program_id  UUID         NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  user_id     UUID         NOT NULL REFERENCES users(id),
  member_type VARCHAR(20)  NOT NULL,                       -- program_admin | mentor
  status      VARCHAR(20),                                 -- invited | requested | pending | active | declined | withdrawn
  email       TEXT,
  created_on  TIMESTAMPTZ  DEFAULT NOW(),
  updated_on  TIMESTAMPTZ  DEFAULT NOW(),
  UNIQUE (program_id, user_id, member_type),
  CONSTRAINT program_members_type_check   CHECK (member_type IN ('program_admin', 'mentor')),
  CONSTRAINT program_members_status_check CHECK (status IS NULL OR status IN ('invited', 'requested', 'pending', 'active', 'declined', 'withdrawn'))
);

-- ============================================
-- TABLE: applications
-- Source: jobspring-prod-program-term-mentees
-- Tracks a user's application and active enrollment lifecycle for a program term.
-- evaluation and reviewer_note are private and exposed only through
-- reviewer-authorized routes.
-- ============================================
CREATE TABLE IF NOT EXISTS applications (
  id                   UUID        PRIMARY KEY,
  program_term_id      UUID        NOT NULL REFERENCES program_terms(id) ON DELETE CASCADE,
  user_id              UUID        NOT NULL REFERENCES users(id),
  role                 VARCHAR(20) NOT NULL DEFAULT 'mentee',   -- mentor | mentee
  status               VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending | accepted | declined | withdrawn | graduated | hold
  program_term_status  VARCHAR(20),                             -- denormalised: open | closed
  start_date_time      TIMESTAMPTZ,
  end_date_time        TIMESTAMPTZ,
  tasks_submitted      BOOLEAN     DEFAULT false,
  admin_notified       BOOLEAN     DEFAULT false,
  attendance_type      VARCHAR(20),                                 -- full_time | part_time (required on accept)
  created_on           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_on           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  evaluation           TEXT,
  reviewer_note        TEXT,
  CONSTRAINT applications_role_check       CHECK (role   IN ('mentor', 'mentee')),
  CONSTRAINT applications_status_check     CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn', 'graduated', 'hold')),
  CONSTRAINT applications_attendance_check CHECK (attendance_type IS NULL OR attendance_type IN ('full_time', 'part_time'))
);

-- A withdrawn application is kept as history beside a reapplication, so only
-- the applications still in play are unique. Withdrawn is terminal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_applications_active
  ON applications (program_term_id, user_id, role)
  WHERE status <> 'withdrawn';

-- ============================================
-- TABLE: tasks
-- Source: jobspring-prod-tasks
-- Every task belongs to an application, its authorization parent.
-- ============================================
CREATE TABLE IF NOT EXISTS tasks (
  id                   UUID        PRIMARY KEY,
  application_id       UUID        NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  program_term_id      UUID        REFERENCES program_terms(id) ON DELETE SET NULL,  -- denormalised for direct lookup
  assignee_id          UUID        NOT NULL REFERENCES users(id),
  owner_id             UUID        REFERENCES users(id),
  name                 TEXT,
  description          TEXT,
  category             VARCHAR(50),                               -- prerequisite | non_prerequisite
  status               VARCHAR(30) NOT NULL DEFAULT 'incomplete', -- incomplete | in_progress | submitted | complete
  application_status   VARCHAR(20),                              -- pending | accepted | declined
  program_term_status  VARCHAR(20),                              -- open | closed
  custom               BOOLEAN     DEFAULT false,
  submit_file          TEXT,                                     -- null | 'required' (flag, not a URL)
  file                 TEXT,                                     -- uploaded file locator
  due_date             TEXT,                                     -- YYYY-MM-DD keeps lexical ORDER BY due_date chronological
  created_by           TEXT,                                     -- lfid of creator
  created_on           TIMESTAMPTZ DEFAULT NOW(),
  updated_on           TIMESTAMPTZ DEFAULT NOW(),
  CONSTRAINT tasks_status_check    CHECK (status   IN ('incomplete', 'in_progress', 'submitted', 'complete')),
  CONSTRAINT tasks_category_check  CHECK (category IS NULL OR category IN ('prerequisite', 'non_prerequisite')),
  CONSTRAINT tasks_due_date_check  CHECK (due_date IS NULL OR due_date ~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$')
);

-- ============================================
-- TABLE: quarantined_tasks
-- Tasks with no application have no authorization parent; held for operator repair.
-- ============================================
CREATE TABLE IF NOT EXISTS quarantined_tasks (
  LIKE tasks INCLUDING DEFAULTS INCLUDING CONSTRAINTS,
  quarantine_reason TEXT        NOT NULL,
  quarantined_on    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (id)
);

-- LIKE copies NOT NULL; a quarantined task is one that has no application.
ALTER TABLE quarantined_tasks ALTER COLUMN application_id DROP NOT NULL;

-- ============================================
-- TABLE: mentorship_approver_team_members
-- Roster behind mentorship_approver_team:global in OpenFGA.
-- ============================================
CREATE TABLE IF NOT EXISTS mentorship_approver_team_members (
  user_id    UUID PRIMARY KEY REFERENCES users(id),
  created_on TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_on TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- TABLE: fga_outbox
-- Transactional queue of OpenFGA access markers, relayed to fga-sync.
-- ============================================
CREATE TABLE IF NOT EXISTS fga_outbox (
  id                 BIGSERIAL PRIMARY KEY,
  marker_kind        TEXT NOT NULL,
  object_type        TEXT NOT NULL,
  object_uid         TEXT NOT NULL,
  relation           TEXT,
  username           TEXT,
  desired_operation  TEXT NOT NULL,
  generation         BIGINT NOT NULL DEFAULT 1,
  state              TEXT NOT NULL DEFAULT 'pending',
  claimed_generation BIGINT,
  claimed_at         TIMESTAMPTZ,
  attempts           INTEGER NOT NULL DEFAULT 0,
  next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error         TEXT,
  created_on         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_on         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT fga_outbox_marker_kind_check
    CHECK (marker_kind IN ('object', 'membership')),
  CONSTRAINT fga_outbox_operation_check
    CHECK (desired_operation IN ('sync', 'remove', 'update_access', 'delete_access')),
  CONSTRAINT fga_outbox_state_check
    CHECK (state IN ('pending', 'in_flight', 'dead_letter')),
  CONSTRAINT fga_outbox_membership_fields_check
    CHECK (
      marker_kind = 'object'
      OR (relation IS NOT NULL AND username IS NOT NULL)
    ),
  CONSTRAINT fga_outbox_object_operation_check
    CHECK (
      (marker_kind = 'object' AND desired_operation IN ('update_access', 'delete_access'))
      OR (marker_kind = 'membership' AND desired_operation IN ('sync', 'remove'))
    )
);

-- ============================================
-- TABLE: index_outbox
-- Transactional queue of search index documents, relayed to the indexer.
-- ============================================
CREATE TABLE IF NOT EXISTS index_outbox (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  object_type     TEXT NOT NULL,
  object_uid      UUID NOT NULL,
  action          TEXT NOT NULL CHECK (action IN ('created', 'updated', 'deleted')),
  headers         JSONB NOT NULL,
  data            JSONB,
  indexing_config JSONB,
  state           TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'in_flight', 'sent', 'dead_letter')),
  generation      BIGINT NOT NULL DEFAULT 1,
  claimed_generation BIGINT,
  attempts        INTEGER NOT NULL DEFAULT 0,
  created_on      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sent_on         TIMESTAMPTZ,
  claimed_at      TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error      TEXT
);

-- ============================================
-- TABLE: object_deletions
-- Transactional queue of stored files to delete. Every path that drops a file
-- locator writes the locator here in the same transaction; a relay deletes the
-- object once the entry is due and no file column still holds the locator.
-- ============================================
CREATE TABLE IF NOT EXISTS object_deletions (
  id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  bucket          TEXT        NOT NULL CHECK (bucket IN ('logos', 'attachments')),
  -- The value the file column held: a CDN URL for logos, an object key for attachments.
  locator         TEXT        NOT NULL CHECK (locator <> ''),
  state           TEXT        NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'in_flight', 'dead_letter')),
  attempts        INTEGER     NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error      TEXT,
  claimed_at      TIMESTAMPTZ,
  created_on      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- TRIGGERS
-- ============================================
CREATE TRIGGER set_updated_on BEFORE UPDATE ON users              FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON user_profiles       FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON programs            FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_skills      FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_funding_stats FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_terms       FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_members     FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON applications        FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON tasks               FOR EACH ROW EXECUTE FUNCTION set_updated_on();

-- ============================================
-- INDEXES
-- ============================================

-- users
CREATE INDEX IF NOT EXISTS idx_users_lfid            ON users(lfid) WHERE lfid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_email           ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_avatar_url      ON users(avatar_url) WHERE avatar_url IS NOT NULL;

-- user_profiles
CREATE INDEX IF NOT EXISTS idx_user_profiles_user_id      ON user_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_profiles_profile_type ON user_profiles(profile_type);
CREATE INDEX IF NOT EXISTS idx_user_profiles_slug         ON user_profiles(slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_profiles_logo_url     ON user_profiles(logo_url) WHERE logo_url IS NOT NULL;
-- A user has one mentee profile; mentor profiles may repeat.
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_profiles_user_type
  ON user_profiles(user_id, profile_type)
  WHERE profile_type = 'mentee';

-- programs
CREATE INDEX IF NOT EXISTS idx_programs_slug           ON programs(slug);
CREATE INDEX IF NOT EXISTS idx_programs_status         ON programs(status);
CREATE INDEX IF NOT EXISTS idx_programs_lfid           ON programs(lfid) WHERE lfid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_programs_lf_project_uid ON programs(lf_project_uid) WHERE lf_project_uid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_programs_lf_project_slug ON programs(lf_project_slug) WHERE lf_project_slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_programs_logo_url       ON programs(logo_url) WHERE logo_url IS NOT NULL;

-- program_skills
CREATE INDEX IF NOT EXISTS idx_program_skills_program_id ON program_skills(program_id);

-- program_funding_stats
CREATE INDEX IF NOT EXISTS idx_program_funding_stats_program_id ON program_funding_stats(program_id);

-- program_terms
CREATE INDEX IF NOT EXISTS idx_program_terms_program_id   ON program_terms(program_id);
CREATE INDEX IF NOT EXISTS idx_program_terms_status       ON program_terms(status);
CREATE INDEX IF NOT EXISTS idx_program_terms_start        ON program_terms(start_date_time);

-- program_members
CREATE INDEX IF NOT EXISTS idx_program_members_program_id ON program_members(program_id);
CREATE INDEX IF NOT EXISTS idx_program_members_user_id    ON program_members(user_id);
CREATE INDEX IF NOT EXISTS idx_program_members_type       ON program_members(member_type);

-- applications
CREATE INDEX IF NOT EXISTS idx_applications_program_term_id ON applications(program_term_id);
CREATE INDEX IF NOT EXISTS idx_applications_user_id         ON applications(user_id);
CREATE INDEX IF NOT EXISTS idx_applications_status          ON applications(status);

-- tasks
CREATE INDEX IF NOT EXISTS idx_tasks_application_id  ON tasks(application_id) WHERE application_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_program_term_id ON tasks(program_term_id) WHERE program_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_assignee_id     ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS idx_tasks_owner_id        ON tasks(owner_id) WHERE owner_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_status          ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_category        ON tasks(category);
CREATE INDEX IF NOT EXISTS idx_tasks_file            ON tasks(file) WHERE file IS NOT NULL;

-- quarantined_tasks
CREATE INDEX IF NOT EXISTS idx_quarantined_tasks_file ON quarantined_tasks(file) WHERE file IS NOT NULL;

-- fga_outbox
CREATE UNIQUE INDEX IF NOT EXISTS uq_fga_outbox_object_marker
  ON fga_outbox(object_type, object_uid)
  WHERE marker_kind = 'object';
CREATE UNIQUE INDEX IF NOT EXISTS uq_fga_outbox_membership_marker
  ON fga_outbox(object_type, object_uid, relation, username)
  WHERE marker_kind = 'membership';
CREATE INDEX IF NOT EXISTS idx_fga_outbox_claimable
  ON fga_outbox(state, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_fga_outbox_object_serialization
  ON fga_outbox(object_type, object_uid, id);

-- index_outbox
CREATE UNIQUE INDEX IF NOT EXISTS uq_index_outbox_object
  ON index_outbox(object_type, object_uid);
CREATE INDEX IF NOT EXISTS idx_index_outbox_pending
  ON index_outbox(next_attempt_at, created_on) WHERE state = 'pending';

-- object_deletions
CREATE INDEX IF NOT EXISTS idx_object_deletions_pending
  ON object_deletions(next_attempt_at) WHERE state = 'pending';
CREATE INDEX IF NOT EXISTS idx_object_deletions_in_flight
  ON object_deletions(claimed_at) WHERE state = 'in_flight';

COMMIT;
