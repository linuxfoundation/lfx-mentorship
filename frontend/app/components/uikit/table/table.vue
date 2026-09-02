<!--
Copyright The Linux Foundation and each contributor to LFX.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    ref="table"
    class="c-table"
    v-bind="$attrs"
  >
    <table>
      <slot />
    </table>
  </div>
</template>

<script lang="ts">
export default {
  name: 'LfxTable',
};
</script>
