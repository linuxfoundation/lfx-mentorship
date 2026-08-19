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
-- Source: jobspring-prod-user-profiles (type = 'mentor' | 'apprentice')
-- Rows where recordKind = 'github-profile-reservation' are excluded.
-- ============================================
CREATE TABLE IF NOT EXISTS user_profiles (
  id                   UUID         PRIMARY KEY,
  user_id              UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  profile_type         TEXT         NOT NULL,              -- mentor | apprentice
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
  status               VARCHAR(20)  NOT NULL DEFAULT 'pending',   -- pending | published | archived
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
  apprentice_needs     JSONB,                              -- {mentors[], skills[], programTerms{}, acceptedMentees, graduatedMentees}
  task_templates       JSONB,                              -- default task list for new terms
  created_on           TIMESTAMPTZ  DEFAULT NOW(),
  updated_on           TIMESTAMPTZ  DEFAULT NOW(),
  CONSTRAINT programs_status_check CHECK (status IN ('pending', 'published', 'archived'))
);

-- ============================================
-- TABLE: program_skills
-- Source: jobspring-prod-projects → apprenticeNeeds.skills[]
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
-- TABLE: invitation_tokens
-- Source: not in DynamoDB (programmatically generated); table created for
-- future use and API compatibility.
-- ============================================
CREATE TABLE IF NOT EXISTS invitation_tokens (
  id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id UUID         NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  token      TEXT         NOT NULL UNIQUE,
  role       VARCHAR(20)  NOT NULL,
  created_on TIMESTAMPTZ  DEFAULT NOW(),
  updated_on TIMESTAMPTZ  DEFAULT NOW(),
  CONSTRAINT invitation_tokens_role_check CHECK (role IN ('mentor', 'mentee'))
);

-- ============================================
-- TABLE: program_terms
-- Source: jobspring-prod-program-terms
-- ============================================
CREATE TABLE IF NOT EXISTS program_terms (
  id                    UUID         PRIMARY KEY,
  program_id            UUID         NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  name                  TEXT         NOT NULL,
  status                VARCHAR(20)  NOT NULL DEFAULT 'open',     -- open | closed
  active_users          INTEGER      DEFAULT 0,
  start_date_time       TIMESTAMPTZ,
  end_date_time         TIMESTAMPTZ,
  application_start_date TIMESTAMPTZ,
  application_end_date   TIMESTAMPTZ,
  created_on            TIMESTAMPTZ  DEFAULT NOW(),
  updated_on            TIMESTAMPTZ  DEFAULT NOW(),
  CONSTRAINT program_terms_status_check CHECK (status IN ('open', 'closed'))
);

-- ============================================
-- TABLE: program_members
-- Source: jobspring-prod-project-members
-- Maintainers and mentors attached to a program (not mentee applicants).
-- ============================================
CREATE TABLE IF NOT EXISTS program_members (
  id          UUID         PRIMARY KEY,
  program_id  UUID         NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  user_id     UUID         NOT NULL REFERENCES users(id),
  member_type VARCHAR(20)  NOT NULL,                       -- maintainer | mentor | apprentice
  status      VARCHAR(20),                                 -- pending | active | graduated | withdrawn
  email       TEXT,
  created_on  TIMESTAMPTZ  DEFAULT NOW(),
  updated_on  TIMESTAMPTZ  DEFAULT NOW(),
  UNIQUE (program_id, user_id, member_type)
);

-- ============================================
-- TABLE: program_admins
-- Source: jobspring-prod-project-members (memberType = 'maintainer')
-- Links a user_profile to a program with an admin role.
-- ============================================
CREATE TABLE IF NOT EXISTS program_admins (
  id              UUID  PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id      UUID  NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  user_profile_id UUID  NOT NULL REFERENCES user_profiles(id),
  role            TEXT  NOT NULL,
  created_on      TIMESTAMPTZ DEFAULT NOW(),
  updated_on      TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE (program_id, user_profile_id)                    -- unique per program+profile
);

-- ============================================
-- TABLE: applications
-- Source: jobspring-prod-program-term-mentees (all status values)
-- Tracks a user's application to a program term before enrollment.
-- ============================================
CREATE TABLE IF NOT EXISTS applications (
  id                   UUID        PRIMARY KEY,
  program_term_id      UUID        NOT NULL REFERENCES program_terms(id) ON DELETE CASCADE,
  user_id              UUID        NOT NULL REFERENCES users(id),
  role                 VARCHAR(20) NOT NULL DEFAULT 'mentee',   -- mentor | mentee
  status               VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending | accepted | declined | withdrawn
  program_term_status  VARCHAR(20),                             -- denormalised: open | closed
  tasks_submitted      BOOLEAN     DEFAULT false,
  admin_notified       BOOLEAN     DEFAULT false,
  created_on           TIMESTAMPTZ DEFAULT NOW(),
  updated_on           TIMESTAMPTZ DEFAULT NOW(),
  CONSTRAINT applications_role_check   CHECK (role   IN ('mentor', 'mentee')),
  CONSTRAINT applications_status_check CHECK (status IN ('pending', 'accepted', 'declined', 'withdrawn')),
  UNIQUE (program_term_id, user_id, role)
);

-- ============================================
-- TABLE: enrollments
-- Source: jobspring-prod-program-term-mentees (status: active | graduated | withdrawn | hold)
-- Represents an accepted mentee who is actively (or was) in a term.
-- ============================================
CREATE TABLE IF NOT EXISTS enrollments (
  id                  UUID        PRIMARY KEY,
  program_term_id     UUID        NOT NULL REFERENCES program_terms(id) ON DELETE CASCADE,
  mentee_user_id      UUID        NOT NULL REFERENCES users(id),
  status              VARCHAR(20) NOT NULL DEFAULT 'active',  -- active | graduated | withdrawn | hold
  program_term_status VARCHAR(20),                            -- denormalised: open | closed
  start_date_time     TIMESTAMPTZ,
  end_date_time       TIMESTAMPTZ,
  tasks_submitted     BOOLEAN     DEFAULT false,
  admin_notified      BOOLEAN     DEFAULT false,
  created_on          TIMESTAMPTZ DEFAULT NOW(),
  updated_on          TIMESTAMPTZ DEFAULT NOW(),
  CONSTRAINT enrollments_status_check CHECK (status IN ('active', 'graduated', 'withdrawn', 'hold')),
  UNIQUE (program_term_id, mentee_user_id)
);

-- ============================================
-- TABLE: tasks
-- Source: jobspring-prod-tasks
-- Tasks are linked to an enrollment (term + assignee). Enrollment is derived
-- during migration via (program_term_id, assignee_id) lookup.
-- ============================================
CREATE TABLE IF NOT EXISTS tasks (
  id                   UUID        PRIMARY KEY,
  enrollment_id        UUID        REFERENCES enrollments(id) ON DELETE SET NULL,
  program_term_id      UUID        REFERENCES program_terms(id),  -- denormalised for direct lookup
  assignee_id          UUID        NOT NULL REFERENCES users(id),
  owner_id             UUID        REFERENCES users(id),
  name                 TEXT,
  description          TEXT,
  category             VARCHAR(50),                               -- prerequisite | milestone
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
  CONSTRAINT tasks_status_check CHECK (status IN ('incomplete', 'in_progress', 'complete', 'submitted'))
);

-- ============================================
-- TRIGGERS
-- ============================================
CREATE TRIGGER set_updated_on BEFORE UPDATE ON users              FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON user_profiles       FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON programs            FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_skills      FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_funding_stats FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON invitation_tokens   FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_terms       FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_members     FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON program_admins      FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON applications        FOR EACH ROW EXECUTE FUNCTION set_updated_on();
CREATE TRIGGER set_updated_on BEFORE UPDATE ON enrollments         FOR EACH ROW EXECUTE FUNCTION set_updated_on();
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

-- program_admins
CREATE INDEX IF NOT EXISTS idx_program_admins_program_id      ON program_admins(program_id);
CREATE INDEX IF NOT EXISTS idx_program_admins_user_profile_id ON program_admins(user_profile_id);

-- applications
CREATE INDEX IF NOT EXISTS idx_applications_program_term_id ON applications(program_term_id);
CREATE INDEX IF NOT EXISTS idx_applications_user_id         ON applications(user_id);
CREATE INDEX IF NOT EXISTS idx_applications_status          ON applications(status);

-- enrollments
CREATE INDEX IF NOT EXISTS idx_enrollments_program_term_id  ON enrollments(program_term_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_mentee_user_id   ON enrollments(mentee_user_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_status           ON enrollments(status);

-- tasks
CREATE INDEX IF NOT EXISTS idx_tasks_enrollment_id   ON tasks(enrollment_id) WHERE enrollment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_program_term_id ON tasks(program_term_id) WHERE program_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_assignee_id     ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS idx_tasks_owner_id        ON tasks(owner_id) WHERE owner_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_status          ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_category        ON tasks(category);

COMMIT;
