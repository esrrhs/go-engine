set -x
cd ./src
for NAME in $(ls ./); do
	echo $NAME
	cd ./$NAME
        rm go.mod -f
	go mod init github.com/esrrhs/go-engine/src/$NAME
	go mod tidy
        cd ..
done

