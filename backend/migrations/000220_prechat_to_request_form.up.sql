-- CF-198: one pre-chat form. The web widget's pre-chat form is a request
-- form (request_forms, CF-52) from now on; the widget settings keep only the
-- switch (prechat.enabled), the chosen form (prechat.form_uuid) and the
-- consent box. Widgets that asked for name / e-mail / phone with their own
-- pre-chat settings get a request form with the same fields when the
-- organization offers none on the widget channel yet.
-- Data-loss note: custom pre-chat questions (prechat.fields) are not
-- carried over; they map to no custom field definition. Their past answers
-- stay on the messages (content_attributes.prechat).
INSERT INTO request_forms (organization_id, name, description, is_active, channels, fields, position)
SELECT src.organization_id, 'Ön sohbet formu', '', TRUE, '{widget}',
       (SELECT COALESCE(jsonb_agg(f), '[]'::jsonb) FROM (
            SELECT jsonb_build_object('key', 'name', 'source', 'system', 'label', 'Ad Soyad', 'help', '',
                                      'required', src.name_mode = 'required', 'required_to_resolve', FALSE) AS f
            WHERE src.name_mode <> 'hidden'
            UNION ALL
            SELECT jsonb_build_object('key', 'email', 'source', 'system', 'label', 'E-posta', 'help', '',
                                      'required', src.email_mode = 'required', 'required_to_resolve', FALSE)
            WHERE src.email_mode <> 'hidden'
            UNION ALL
            SELECT jsonb_build_object('key', 'phone', 'source', 'system', 'label', 'Telefon', 'help', '',
                                      'required', src.phone_mode = 'required', 'required_to_resolve', FALSE)
            WHERE src.phone_mode <> 'hidden'
       ) fields),
       0
FROM (
    SELECT DISTINCT ON (ci.organization_id) ci.organization_id,
           COALESCE(ci.settings->'prechat'->>'name', 'optional') AS name_mode,
           COALESCE(ci.settings->'prechat'->>'email', 'optional') AS email_mode,
           COALESCE(ci.settings->'prechat'->>'phone', 'hidden') AS phone_mode
    FROM channel_instances ci
    WHERE ci.channel_type = 'widget' AND ci.deleted_at IS NULL
      AND COALESCE((ci.settings->'prechat'->>'enabled')::boolean, TRUE)
    ORDER BY ci.organization_id, ci.id
) src
WHERE NOT EXISTS (
    SELECT 1 FROM request_forms rf
    WHERE rf.organization_id = src.organization_id AND rf.is_active AND 'widget' = ANY (rf.channels)
)
AND (src.name_mode <> 'hidden' OR src.email_mode <> 'hidden' OR src.phone_mode <> 'hidden');
