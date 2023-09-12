import Vue from 'vue';
import { mapGetters } from 'vuex';
import { getAlertConditionChoices, getAlertCondition } from '@pkg/opni/utils/requests/alerts';
import { Cluster } from '@pkg/opni/models/Cluster';

export async function loadChoices(parent: Vue, typeAsString: string, typeAsEnum: number) {
  try {
    const allChoices = await getAlertConditionChoices({ alertType: typeAsEnum });
    const choices = (allChoices as any)[typeAsString];

    parent.$set(parent, 'choices', { ...choices });
  } catch (ex) { }
}

export function mapClusterOptions() {
  return {
    ...mapGetters({ clusters: 'opni/clusters' }),
    clusterOptions() {
      return this.clusters.map((c: Cluster) => ({
        label: c.nameDisplay,
        value: c.id
      }));
    }
  };
}

export function createConditionRequest(vue: any, route: any) {
  return route.params.id && route.params.id !== 'create' ? getAlertCondition({ id: route.params.id, groupId: route.query.groupId }, vue) : Promise.resolve(false);
}
