import Vue from 'vue';
import { getAlertConditionChoices } from '../../utils/requests/alerts';
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
