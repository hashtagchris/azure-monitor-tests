-- Moves OTLP-specific fields (severity_text, trace_id, span_id) from the
-- parsed log record into chunk metadata under metadata.otlp.*, where
-- out_opentelemetry reads them via its $otlp['...'] record accessors.
-- The output plugin skips the top-level `otlp` metadata key when packing
-- remaining metadata as OTLP attributes, which prevents these fields
-- from being duplicated as attributes (the issue with the flat-metadata
-- content_modifier approach).
function add_otlp_metadata(tag, ts, group, metadata, record)
    if metadata == nil then
        metadata = {}
    end

    local otlp = metadata.otlp or {}
    local keys = {"severity_text", "trace_id", "span_id"}
    for _, key in ipairs(keys) do
        if record[key] ~= nil then
            otlp[key] = record[key]
            record[key] = nil
        end
    end
    metadata.otlp = otlp

    -- return code 2 = modified record, keep original timestamp
    return 2, ts, metadata, record
end
