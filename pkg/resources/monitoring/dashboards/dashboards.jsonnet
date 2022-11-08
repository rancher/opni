local dashboards = (import 'mixin.libsonnet').grafanaDashboards;


local patchDashboard(d) = {
  templating: {
    list: [
      if x.name == 'cluster'
      then x {
        query: 'query_result(opni_cluster_info)',
        regex: '/cluster_id="(?<value>[^"]+)|friendly_name="(?<text>[^"]+)/g',
      }
      else x
      for x in d.templating.list
    ],
  },
};

{
  [name]: dashboards[name] + patchDashboard(dashboards[name])
  for name in std.objectFields(dashboards)
}
