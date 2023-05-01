<script>
import jsyaml from 'js-yaml';
import YamlEditor from '@shell/components/YamlEditor';
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';
import { exceptionToErrorsArray } from '../utils/error';
import { getGatewayConfig, updateGatewayConfig } from '../utils/requests/management';

export default {
  components: {
    YamlEditor,
    AsyncButton,
    Loading,
    Banner
  },

  async fetch() {
    await this.load();
  },

  data() {
    return {
      loading:          false,
      documents:        [],
      editorContents:   '',
      error:           '',
    };
  },

  methods: {
    async load() {
      try {
        this.loading = true;
        this.$set(this, 'documents', await getGatewayConfig(this));
        let contents = '';

        this.documents.forEach((doc, i) => {
          contents += doc.yaml;
          if (i < this.documents.length - 1) {
            if (contents[contents.length - 1] !== '\n') {
              contents += '\n';
            }
            contents += '---\n';
          }
        });
        this.$set(this, 'editorContents', contents);
      } finally {
        this.loading = false;
      }
    },
    async save(buttonCallback) {
      try {
        const documents = jsyaml.loadAll(this.editorContents);
        const jsonDocuments = [];

        documents.forEach((doc) => {
          jsonDocuments.push(JSON.stringify(doc));
        });
        await updateGatewayConfig(jsonDocuments);
      } catch (err) {
        buttonCallback(false);
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      } finally {
        buttonCallback(!this.error);
      }
    },
    async reset() {
      this.$set(this, 'error', '');
      await this.load();
    },
  }
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header class="m-0">
      <div class="title">
        <h1>Configuration</h1>
      </div>
    </header>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
    <div slot="body">
      <YamlEditor
        v-model="editorContents"
        :as-object="false"
        :editor-mode="'EDIT_CODE'"
        :read-only="false"
        class="yaml-editor"
      />
      <div class="row actions-container mt-10">
        <button class="btn role-secondary mr-10" @click="reset">
          Reset
        </button>
        <AsyncButton
          class="btn role-primary mr-10"
          action-label="Save and Restart"
          waiting-label="Saving..."
          success-label="Restarting..."
          error-label="Error"
          @click="save"
        />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.actions-container {
  display: flex;
  justify-content: flex-end;
}
.buttons {
  display: flex;
  flex-direction: row;
  justify-content: flex-end;
  width: 100%;
}
.error-message {
  color: var(--error);
  display: flex;
  justify-content: center;
  align-items: center;
}
</style>
