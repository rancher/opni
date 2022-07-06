from setuptools import find_packages, setup

setup(
  name="opni_proto",
  version="0.5.4",
  install_requires=["betterproto~=1.2.0"],
  packages=find_packages(),
)
