FROM python:3

RUN pip3 install --quiet bravado crypto pytest twisted requests pyyaml pymongo tz

RUN mkdir -p /testing

COPY . /testing/

ENTRYPOINT ["bash", "/testing/run.sh"]
