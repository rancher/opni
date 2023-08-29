import Vue from 'vue';
import { getAlertConditionChoices, getAlertCondition } from '../../utils/requests/alerts';
import { getClusters } from '../../utils/requests/management';

export async function loadChoices(parent: Vue, typeAsString: string, typeAsEnum: number) {
  try {
    const allChoices = await getAlertConditionChoices({ alertType: typeAsEnum });
    const choices = (allChoices as any)[typeAsString];

    parent.$set(parent, 'choices', { ...choices });
  } catch (ex) { }
}

export async function loadClusters(parent: Vue) {
  const clusters = await getClusters(parent);

  parent.$set(parent, 'clusterOptions', clusters.map(c => ({
    label: c.nameDisplay,
    value: c.id
  })));
}

export function createConditionRequest(this: any, route: any) {
  return route.params.id && route.params.id !== 'create' ? getAlertCondition({ id: route.params.id, groupId: route.query.groupId }, this) : Promise.resolve(false);
}
