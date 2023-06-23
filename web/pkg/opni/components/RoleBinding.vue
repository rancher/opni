<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import AsyncButton from '@shell/components/AsyncButton';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import ArrayList from '@shell/components/form/ArrayList';
import { Banner } from '@components/Banner';
import { exceptionToErrorsArray } from '../utils/error';
import { createRoleBinding, getRoles } from '../utils/requests/management';

export default {
  components: {
    AsyncButton,
    LabeledInput,
    LabeledSelect,
    Loading,
    Tab,
    Tabbed,
    ArrayList,
    Banner,
  },

  async fetch() {
    const roles = await getRoles();

    this.$set(this, 'roles', roles);
  },

  data() {
    return {
      name:      '',
      roleName:  '',
      subjects:  [],
      roles:     [],
      error:    '',
    };
  },

  methods: {
    async save(buttonCallback) {
      if (this.name === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }
      if (this.roleName === '') {
        this.$set(this, 'error', 'Role is required');
        buttonCallback(false);

        return;
      }
      try {
        await createRoleBinding(this.name, this.roleName, this.subjects);
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);

        return;
      }
      this.$set(this, 'error', '');
      buttonCallback(true);
      this.$router.replace({ name: 'role-bindings' });
    },

    cancel() {
      this.$router.replace({ name: 'role-bindings' });
    }
  },

  computed: {
    roleOptions() {
      return this.roles.map(role => ({
        label: role.name,
        value: role.id
      }));
    }
  }
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <div class="row">
      <div class="col span-6">
        <LabeledInput
          v-model="name"
          label="Name"
          :required="true"
        />
      </div>
      <div class="col span-6">
        <LabeledSelect
          v-model="roleName"
          class="mb-20"
          :label="t('opni.monitoring.roleBindings.role')"
          :options="roleOptions"
          :required="true"
        />
      </div>
    </div>
    <Tabbed :side-tabs="true" class="mb-20">
      <Tab
        name="subjects"
        c
        :label="t('opni.monitoring.roleBindings.tabs.subjects.label')"
        :weight="2"
      >
        <ArrayList
          v-model="subjects"
          :protip="false"
          add-label="Add Subject"
        />
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn btn-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <AsyncButton
        mode="edit"
        @click="save"
      />
    </div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
  </div>
</template>

<style lang="scss" scoped>
.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}

.install-command {
  width: 100%;
}

::v-deep .warning {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}
</style>
