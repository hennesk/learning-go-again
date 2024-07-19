docker container rm -f $(docker container ls -aq -f name=identity-app)
docker image rm -f $(docker image ls -aq -f reference=identity-app)