-- ACL policy alignment (audit L2). The 0001 memex_can_read treated any
-- non-array `read` value as OPEN (jsonb_typeof <> 'array' short-circuited
-- the whole grant), so a malformed ACL stored before the admin endpoint
-- started validating (note.ValidACL) opened the space to every agent on
-- the search path while the Go policy (note.Can) denied it. The two
-- implementations now agree, fail closed on the same shapes:
--
--   - JSON null is equivalent to absent (memexctl sends {"write":null}).
--   - A non-array read or write value fails closed for everyone.
--   - The writer-fallback grant applies only to non-empty write arrays,
--     matching note.Can (an empty/absent write list is the default-open
--     write convention, not a read grant).
--
-- Validated shapes: {} open, null open, {"read":["a","b"],"write":null}
-- denies strangers / allows listed readers, {"read":"x"} deny,
-- {"write":5} deny, {"read":[]} open, {"read":["*"]} open.

CREATE OR REPLACE FUNCTION memex_can_read(acl jsonb, agent text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  WITH parts AS (
    SELECT
      CASE WHEN NOT (acl ? 'read')  OR jsonb_typeof(acl->'read')  = 'null' THEN NULL ELSE acl->'read'  END AS r,
      CASE WHEN NOT (acl ? 'write') OR jsonb_typeof(acl->'write') = 'null' THEN NULL ELSE acl->'write' END AS w
  )
  SELECT
    acl IS NULL
    OR acl = '{}'::jsonb
    OR (
      (parts.r IS NULL OR jsonb_typeof(parts.r) = 'array')
      AND (parts.w IS NULL OR jsonb_typeof(parts.w) = 'array')
      AND (
        parts.r IS NULL
        OR CASE WHEN jsonb_typeof(parts.r) = 'array'
             THEN jsonb_array_length(parts.r) = 0 OR parts.r ? '*' OR parts.r ? agent
             ELSE false END
        OR CASE WHEN parts.w IS NOT NULL AND jsonb_typeof(parts.w) = 'array'
             THEN jsonb_array_length(parts.w) > 0 AND (parts.w ? agent OR parts.w ? '*')
             ELSE false END
      )
    )
  FROM parts
$$;
