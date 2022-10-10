from setuptools import find_packages, setup
import os

setup(
  name="opni_proto",
  version="0.6.1.0",
  install_requires=["betterproto~=1.2.0"],
  packages=find_packages(),
)
