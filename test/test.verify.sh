OUTPUT_LOG="./run/logs/output.log"

echo "Validating ${1} test"

if [ "$1" = "success" ]; then
	if ! grep -q "X-Is-Trusted: yes" $OUTPUT_LOG; then
		echo "'X-Is-Trusted: yes' header was not added and parsed"
		exit 5
	fi
	if ! grep -q "X-Forwarded-For: ${2}" $OUTPUT_LOG; then
		echo "'X-Forwarded-For: ${2}' header not defined"
		exit 5
	fi
	if ! grep -q "X-Real-Ip: ${2}" $OUTPUT_LOG; then
		echo "'X-Real-Ip: ${2}' header not defined"
		exit 5
	fi
	if ! grep -q "Cf-Visitor: {\"scheme\":\"https\"}" $OUTPUT_LOG; then
		echo "'Cf-Visitor: {\"scheme\":\"https\"}' header not defined"
		exit 5
	fi
elif [ "$1" = "fail" ]; then
	if ! grep -q "X-Is-Trusted: no" $OUTPUT_LOG; then
		echo "'X-Is-Trusted: no' header was not added"
		exit 5
	fi
	# if ! grep -q "X-Forwarded-For: 10.0.0.2" $OUTPUT_LOG; then
	# 	echo "Forwarded header was not defined as the original IP"
	# 	exit 5
	# fi
elif [ "$1" = "invalid" ]; then
	if ! grep -q "X-Is-Trusted: no" $OUTPUT_LOG; then
		echo "'X-Is-Trusted: no' header was not added"
		exit 5
	fi
else
	echo "Error, unknown test type ${1}"
	exit 10
fi

echo "Test OK"
