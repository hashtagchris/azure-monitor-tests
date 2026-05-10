-- Adds OTLP metadata fields under metadata.otlp.* so out_opentelemetry can
-- read them via its $otlp[...] record accessors. The output plugin skips
-- the top-level `otlp` metadata key when packing remaining metadata as
-- OTLP attributes, which prevents these fields from being duplicated as
-- attributes (the issue with the flat-metadata content_modifier approach).
function add_otlp_metadata(tag, ts, group, metadata, record)
    if metadata == nil then
        metadata = {}
    end
    metadata.otlp = {
        severity_text = "Info",
        trace_id = "5b8efff798038103d269b633813fc60c",
        span_id = "eee19b7ec3c1b173"
    }
    -- return code 2 = modified record, keep original timestamp
    return 2, ts, metadata, record
end
