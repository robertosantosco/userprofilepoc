-- =========================================================
-- Stored Procedures for Entity Graph JSON Generation
-- =========================================================

-- Helper function to format temporal range in human-readable format
CREATE OR REPLACE FUNCTION format_temporal_range(
    period tstzrange,
    granularity text
) RETURNS text AS $$
DECLARE
    start_time timestamptz;
    end_time timestamptz;
    duration interval;
    days_diff int;
BEGIN
    start_time := lower(period);
    end_time := upper(period);

    -- Handle null end time (currently valid)
    IF end_time IS NULL THEN
        CASE granularity
            WHEN 'instant' THEN
                RETURN 'instant @ ' || to_char(start_time, 'YYYY-MM-DD HH24:MI');
            WHEN 'day' THEN
                RETURN 'day @ ' || to_char(start_time, 'DD/MM/YYYY');
            WHEN 'month' THEN
                RETURN 'month @ ' || to_char(start_time, 'MM/YYYY');
            WHEN 'year' THEN
                RETURN 'year @ ' || to_char(start_time, 'YYYY');
            ELSE
                RETURN 'instant @ ' || to_char(start_time, 'YYYY-MM-DD HH24:MI');
        END CASE;
    END IF;

    -- Calculate duration for closed periods
    duration := end_time - start_time;
    days_diff := EXTRACT(DAYS FROM duration);

    -- Format based on granularity and duration
    CASE granularity
        WHEN 'instant' THEN
            IF days_diff = 0 THEN
                RETURN 'instant @ ' || to_char(start_time, 'YYYY-MM-DD HH24:MI');
            ELSE
                RETURN days_diff + 1 || ' days @ from ' ||
                       to_char(start_time, 'YYYY-MM-DD') || ' to ' ||
                       to_char(end_time, 'YYYY-MM-DD');
            END IF;
        WHEN 'day' THEN
            IF days_diff <= 1 THEN
                RETURN 'day @ ' || to_char(start_time, 'DD/MM/YYYY');
            ELSE
                RETURN days_diff + 1 || ' days @ from ' ||
                       to_char(start_time, 'DD/MM/YYYY') || ' to ' ||
                       to_char(end_time, 'DD/MM/YYYY');
            END IF;
        WHEN 'month' THEN
            -- Check if it's a complete month (start is first day, end is first day of next month)
            IF date_trunc('month', start_time) = start_time
               AND date_trunc('month', end_time) = date_trunc('month', start_time) + interval '1 month' THEN
                -- Calculate number of months
                DECLARE
                    month_count int;
                BEGIN
                    month_count := EXTRACT(YEAR FROM age(end_time, start_time)) * 12 +
                                   EXTRACT(MONTH FROM age(end_time, start_time));

                    IF month_count = 1 THEN
                        RETURN '1 month @ ' || to_char(start_time, 'YYYY-MM');
                    ELSE
                        RETURN month_count || ' months @ from ' ||
                               to_char(start_time, 'YYYY-MM') || ' to ' ||
                               to_char(end_time - interval '1 day', 'YYYY-MM');
                    END IF;
                END;
            ELSE
                RETURN days_diff + 1 || ' days @ from ' ||
                       to_char(start_time, 'YYYY-MM-DD') || ' to ' ||
                       to_char(end_time, 'YYYY-MM-DD');
            END IF;
        WHEN 'year' THEN
            -- Check if it's exactly one year
            IF date_trunc('year', start_time) + interval '1 year' = end_time THEN
                RETURN 'year @ ' || to_char(start_time, 'YYYY');
            ELSE
                RETURN days_diff + 1 || ' days @ from ' ||
                       to_char(start_time, 'YYYY-MM-DD') || ' to ' ||
                       to_char(end_time, 'YYYY-MM-DD');
            END IF;
        ELSE
            RETURN days_diff + 1 || ' days @ from ' ||
                   to_char(start_time, 'YYYY-MM-DD') || ' to ' ||
                   to_char(end_time, 'YYYY-MM-DD');
    END CASE;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_entity(
    p_entity_id       int,
    as_of_time        timestamptz DEFAULT now(),
    history_lookback  interval    DEFAULT '90 days',
    history_limit     int         DEFAULT 5,
    max_depth         int         DEFAULT 5,
    p_current_depth   int         DEFAULT 0,
    p_visited_ids     int[]       DEFAULT ARRAY[]::int[]
) RETURNS jsonb AS
$$
DECLARE
    entity_rec   record;
    edge_rec     record;
    result       jsonb;
    edges_array  jsonb[];
    temp_rec     record;
    child_json   jsonb;
BEGIN
    -- Stop conditions: max depth or circular reference
    IF p_current_depth >= max_depth OR p_entity_id = ANY(p_visited_ids) THEN
        RETURN NULL;
    END IF;

    -- Get entity
    SELECT * INTO entity_rec FROM entities WHERE id = p_entity_id;
    IF NOT FOUND THEN
        RETURN jsonb_build_object('error', 'Entity not found');
    END IF;

    -- Start with base entity fields
    result := jsonb_build_object(
        'id',         entity_rec.id,
        'reference',  entity_rec.reference,
        'type',       entity_rec.type,
        'created_at', entity_rec.created_at,
        'updated_at', entity_rec.updated_at
    );

    -- Merge entity properties at root level
    IF entity_rec.properties IS NOT NULL THEN
        result := result || entity_rec.properties;
    END IF;

    -- Add temporal properties with history directly at root
    FOR temp_rec IN
        SELECT tp.key,
               jsonb_agg(
                   jsonb_build_object(
                       'range', format_temporal_range(tp.period, tp.granularity),
                       'items', CASE
                           WHEN jsonb_typeof(tp.value) = 'array' THEN tp.value
                           ELSE jsonb_build_array(tp.value)
                       END
                   ) ORDER BY lower(tp.period) DESC
               ) as temporal_values
        FROM temporal_properties tp
        WHERE tp.entity_id = p_entity_id
          AND lower(tp.period) <= as_of_time
          AND lower(tp.period) >= (as_of_time - history_lookback)
        GROUP BY tp.key
    LOOP
        -- Add each temporal key directly to root, limiting history
        result := result || jsonb_build_object(
            temp_rec.key,
            (SELECT jsonb_agg(val)
             FROM (
                 SELECT val
                 FROM jsonb_array_elements(temp_rec.temporal_values) val
                 LIMIT history_limit
             ) sub)
        );
    END LOOP;

    -- Recursively process edges
    FOR edge_rec IN
        SELECT ed.relationship_type,
               ed.metadata,
               ed.right_entity as child_id
        FROM edges ed
        WHERE ed.left_entity = p_entity_id
        ORDER BY ed.relationship_type, ed.right_entity
    LOOP
        -- Recursively get child entity
        child_json := get_entity(
            edge_rec.child_id,
            as_of_time,
            history_lookback,
            history_limit,
            max_depth,
            p_current_depth + 1,
            p_visited_ids || p_entity_id
        );

        -- Only add if child exists and isn't null
        IF child_json IS NOT NULL THEN
            edges_array := edges_array || jsonb_build_object(
                'type',   edge_rec.relationship_type,
                'entity', child_json
            );
        END IF;
    END LOOP;

    -- Add edges if any exist
    IF array_length(edges_array, 1) > 0 THEN
        result := result || jsonb_build_object('edges', to_jsonb(edges_array));
    END IF;

    RETURN result;
END;
$$ LANGUAGE plpgsql;