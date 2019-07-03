FROM python:3

RUN pip3 install --quiet bravado==9.2.2 crypto pytest==2.8.7 twisted requests pyyaml pymongo tz

RUN mkdir -p /testing

COPY . /testing/

ENTRYPOINT ["bash", "/testing/run.sh"]
