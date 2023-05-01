import { IComponentApi } from '@shell/core/component-api';

// 3. Declare augmentation for Vue
interface Vue {
    $api: IComponentApi;
}
