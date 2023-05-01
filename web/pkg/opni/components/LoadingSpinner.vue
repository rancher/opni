<script>
</script>
<template>
  <div class="initial-load-spinner-container">
    <i class="initial-load-spinner"></i>
  </div>
</template>

<style lang="scss" scoped>
.initial-load-spinner-container {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 100%;
}

.initial-load-spinner {
  animation: initial-load-animate 1s infinite linear;
  box-sizing: border-box;
  border: 2px solid #dcdee7;
  border-radius: 50%;
  border-top-color: #f4f5fa;
  display: inline-block;
  height: 40px;
  margin: 0 auto;
  width: 40px;
}

@keyframes initial-load-animate {
  0% {
    transform: rotate(0deg);
  }

  100% {
    transform: rotate(359deg);
  }
}
</style>
