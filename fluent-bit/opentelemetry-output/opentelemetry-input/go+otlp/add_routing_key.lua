function add_routing_key(tag, ts, group, metadata, record)
    if group == nil or group.resource == nil or group.resource.attributes == nil or group.resource.attributes["service.name"] == nil then
        print("service.name is not set in group resource attributes, skipping routing key addition")
        -- return code 0 = no modification
        return 0, ts, metadata, record
    end

    -- Using underscore as a separator because there's no good way to escape periods in rewrite_tag rules
    record["service_name"] = group.resource.attributes["service.name"]

    -- return code 1 = modified record
    return 1, ts, metadata, record
end
