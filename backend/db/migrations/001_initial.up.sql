-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT
-- ============================================
-- Migration: Mentorship Schema — Initial
-- Source: jobspring-prod-* DynamoDB tables
-- ============================================

BEGIN;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

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
-- ============================================
CREATE TABLE IF NOT EXISTS programs (
  id                   UUID         PRIMARY KEY,
  name                 TEXT         NOT NULL,
  slug                 TEXT         NOT NULL UNIQUE,
  status               VARCHAR(20)  NOT NULL DEFAULT 'draft',    -- draft | submitted | published | rejected | archived | hidden
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
  CONSTRAINT programs_status_check CHECK (status IN ('draft', 'submitted', 'published', 'rejected', 'archived', 'hidden'))
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
-- program_admin member_type replaces the former program_admins table.
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
-- ============================================
CREATE TABLE IF NOT EXISTS applications (
  id                   UUID        PRIMARY KEY,
  program_term_id      UUID        NOT NULL REFERENCES program_terms(id) ON DELETE CASCADE,
  user_id              UUID        NOT NULL REFERENCES users(id),
  role                 VARCHAR(20) NOT NULL DEFAULT 'mentee',   -- mentor | mentee
  status               VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending | accepted | active | declined | withdrawn | graduated | hold
  program_term_status  VARCHAR(20),                             -- denormalised: open | closed
  start_date_time      TIMESTAMPTZ,
  end_date_time        TIMESTAMPTZ,
  tasks_submitted      BOOLEAN     DEFAULT false,
  admin_notified       BOOLEAN     DEFAULT false,
  attendance_type      VARCHAR(20),                                 -- full_time | part_time (required on accept)
  created_on           TIMESTAMPTZ DEFAULT NOW(),
  updated_on           TIMESTAMPTZ DEFAULT NOW(),
  CONSTRAINT applications_role_check       CHECK (role   IN ('mentor', 'mentee')),
  CONSTRAINT applications_status_check     CHECK (status IN ('pending', 'accepted', 'active', 'declined', 'withdrawn', 'graduated', 'hold')),
  CONSTRAINT applications_attendance_check CHECK (attendance_type IS NULL OR attendance_type IN ('full_time', 'part_time')),
  UNIQUE (program_term_id, user_id, role)
);

-- (enrollments table removed: applications now serve the full lifecycle)

-- ============================================
-- TABLE: tasks
-- Source: jobspring-prod-tasks
-- Tasks are linked to an application (term + user). Application is derived
-- during migration via (program_term_id, assignee_id) lookup.
-- ============================================
CREATE TABLE IF NOT EXISTS tasks (
  id                   UUID        PRIMARY KEY,
  application_id       UUID        REFERENCES applications(id) ON DELETE SET NULL,
  program_term_id      UUID        REFERENCES program_terms(id) ON DELETE SET NULL,  -- denormalised for direct lookup
  assignee_id          UUID        NOT NULL REFERENCES users(id),
  owner_id             UUID        REFERENCES users(id),
  name                 TEXT,
  description          TEXT,
  category             VARCHAR(50),                               -- prerequisite | non_prerequisite
  status               VARCHAR(30) NOT NULL DEFAULT 'incomplete', -- incomplete | in_progress | complete | submitted
  application_status   VARCHAR(20),                              -- pending | accepted | declined
  program_term_status  VARCHAR(20),                              -- open | closed
  custom               BOOLEAN     DEFAULT false,
  submit_file          TEXT,                                     -- null | 'required' | URL
  file                 TEXT,                                     -- uploaded file URL
  due_date             DATE,
  created_by           TEXT,                                     -- lfid of creator
  created_on           TIMESTAMPTZ DEFAULT NOW(),
  updated_on           TIMESTAMPTZ DEFAULT NOW(),
  CONSTRAINT tasks_status_check    CHECK (status   IN ('incomplete', 'in_progress', 'complete', 'submitted')),
  CONSTRAINT tasks_category_check  CHECK (category IS NULL OR category IN ('prerequisite', 'non_prerequisite'))
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

-- user_profiles
CREATE INDEX IF NOT EXISTS idx_user_profiles_user_id      ON user_profiles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_profiles_profile_type ON user_profiles(profile_type);
CREATE INDEX IF NOT EXISTS idx_user_profiles_slug         ON user_profiles(slug) WHERE slug IS NOT NULL;

-- programs
CREATE INDEX IF NOT EXISTS idx_programs_slug         ON programs(slug);
CREATE INDEX IF NOT EXISTS idx_programs_status       ON programs(status);
CREATE INDEX IF NOT EXISTS idx_programs_lfid         ON programs(lfid) WHERE lfid IS NOT NULL;

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

COMMIT;
