-- SPDX-License-Identifier: Apache-2.0
--
-- Seed data for developing and demonstrating the database ownership ingest.
--
-- Deliberately shaped like a system of record rather than like CMM's import
-- format: people in one table, the things they are responsible for in another,
-- joined on a staff id. An administrator writes the SELECT that bridges the
-- two, which is the whole point of accepting a query rather than a table.
--
-- The awkward cases are on purpose:
--   * a person who has left the company, whose rows the query filters out
--   * an asset with no owner at all
--   * an owner with no email address
--   * a date column, an NVARCHAR column and a BIT column
--
-- All names are fictional. Never point this at anything real.
--
--   docker compose --profile mssql up -d mssql
--   make seed-mssql

IF DB_ID('cmdb') IS NULL
    CREATE DATABASE cmdb;
GO

USE cmdb;
GO

DROP TABLE IF EXISTS asset_owner;
DROP TABLE IF EXISTS staff;
GO

CREATE TABLE staff (
    staff_id     INT PRIMARY KEY,
    full_name    NVARCHAR(120) NOT NULL,
    email        NVARCHAR(200) NULL,
    team         NVARCHAR(80)  NULL,
    left_company BIT NOT NULL DEFAULT 0
);

CREATE TABLE asset_owner (
    asset_id    INT IDENTITY(1,1) PRIMARY KEY,
    asset_kind  NVARCHAR(40)  NOT NULL,
    asset_name  NVARCHAR(200) NOT NULL,
    staff_id    INT NULL,
    recorded_on DATE NOT NULL
);
GO

INSERT INTO staff (staff_id, full_name, email, team, left_company) VALUES
    (101, N'Priya Raman',   N'priya.raman@example-corp.com',  N'Platform',   0),
    (102, N'Thomas Smith',  N'thomas.smith@example-corp.com', N'Middleware', 0),
    (103, N'Renee Dubois',  N'renee.dubois@example-corp.com', N'Windows',    0),
    (104, N'Alice Jones',   N'alice.jones@example-corp.com',  N'Platform',   1),
    (105, N'Unnamed Owner', NULL,                             NULL,          0);

INSERT INTO asset_owner (asset_kind, asset_name, staff_id, recorded_on) VALUES
    (N'node',     N'homekube001.home.arpa', 101,  '2026-01-15'),
    (N'node',     N'homekube002.home.arpa', 101,  '2026-01-15'),
    (N'node',     N'nexus.home.arpa',       102,  '2026-02-01'),
    (N'node',     N'repo.home.arpa',        102,  '2026-02-01'),
    (N'node',     N'win11-001.home.arpa',   103,  '2026-03-10'),
    (N'cookbook', N'nginx',                 101,  '2026-01-20'),
    (N'cookbook', N'logrotate',             102,  '2026-01-20'),
    (N'git_repo', N'cron',                  103,  '2026-02-14'),
    (N'git_repo', N'kubernetes-cluster',    104,  '2026-02-14'),
    (N'node',     N'orphan-host-01',        NULL, '2026-04-01'),
    (N'node',     N'unmatched-host-99',     105,  '2026-04-02');
GO

-- The query an administrator would write against this, and the one the
-- functional tests use:
--
--   SELECT s.email AS owner_name, a.asset_kind AS entity_type,
--          a.asset_name AS entity_key, s.full_name AS display_name,
--          s.team AS team, a.recorded_on AS recorded_on
--   FROM asset_owner a
--   LEFT JOIN staff s ON s.staff_id = a.staff_id
--   WHERE s.left_company = 0 OR s.left_company IS NULL
--   ORDER BY a.asset_id
GO

-- A login whose password contains the characters that stop a connection string
-- being parsed as a URL: a percent sign, a semicolon, a space and a hash.
--
-- It exists so the encoding repair can be proved by connecting rather than
-- asserted. A customer's connection was refused by the driver as "invalid URL
-- format" with every visible part of it legal, and the password could not be
-- retyped because nobody at hand knew it.
--
-- This is a fixture in a throwaway development container, not a credential. It
-- is read-only on the sample database and exists nowhere else.
IF SUSER_ID(N'cmmnasty') IS NULL
    CREATE LOGIN cmmnasty WITH PASSWORD = N'pa%ss;wo rd#7Q!', CHECK_POLICY = OFF;
GO
USE cmdb;
GO
IF DATABASE_PRINCIPAL_ID(N'cmmnasty') IS NULL
    CREATE USER cmmnasty FOR LOGIN cmmnasty;
GO
ALTER ROLE db_datareader ADD MEMBER cmmnasty;
GO
